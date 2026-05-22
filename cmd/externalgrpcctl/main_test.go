package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/externalgrpc/protos"
)

func TestBuildCall(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "list", args: []string{"list-nodegroups"}},
		{name: "template", args: []string{"template", "--nodegroup", "ng-1"}},
		{name: "node-info", args: []string{"node-info", "--nodegroup", "ng-1"}},
		{name: "scale-up", args: []string{"scale-up", "--nodegroup", "ng-1", "--delta", "2"}},
		{name: "node-delete by name", args: []string{"node-delete", "--nodegroup", "ng-1", "--node", "node-a"}},
		{name: "node-delete by provider id", args: []string{"node-delete", "--nodegroup", "ng-1", "--provider-id", "aws:///us-east-1a/i-123"}},
		{name: "invalid", args: []string{"oops"}, wantErr: true},
		{name: "node-delete missing target", args: []string{"node-delete", "--nodegroup", "ng-1"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call, err := buildCall(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildCall error: %v", err)
			}
			if call.run == nil {
				t.Fatalf("expected runnable call")
			}
		})
	}
}

func TestWithOutgoingHeaders(t *testing.T) {
	ctx := context.Background()

	_, err := withOutgoingHeaders(ctx, []string{"bad-header"})
	if err == nil {
		t.Fatalf("expected parse error")
	}

	newCtx, err := withOutgoingHeaders(ctx, []string{"x-test:abc", "x-test:def"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if newCtx == nil {
		t.Fatalf("expected context")
	}
}

func TestPrintResponseTemplateNode(t *testing.T) {
	node := &corev1.Node{}
	node.Name = "ng-1-template"
	node.Labels = map[string]string{"topology.kubernetes.io/zone": "us-east-1a"}

	nodeBytes, err := node.Marshal()
	if err != nil {
		t.Fatalf("marshal node: %v", err)
	}

	resp := &protos.NodeGroupTemplateNodeInfoResponse{NodeBytes: nodeBytes}

	got := captureStdout(t, func() {
		if err := printResponse(resp); err != nil {
			t.Fatalf("printResponse error: %v", err)
		}
	})

	if !bytes.Contains([]byte(got), []byte(`"name": "ng-1-template"`)) {
		t.Fatalf("expected decoded node name in output, got: %s", got)
	}
	if bytes.Contains([]byte(got), []byte(`"nodeBytes"`)) {
		t.Fatalf("expected decoded node output, got raw nodeBytes field: %s", got)
	}
}

func TestPrintResponseListNodeGroups(t *testing.T) {
	resp := listNodeGroupsResponse{
		NodeGroups: []nodeGroupSummary{
			{
				ID:          "ng-1",
				MinSize:     1,
				MaxSize:     8,
				CurrentSize: 3,
				DesiredSize: 4,
				Debug:       "ng-1 (1:8)",
			},
		},
	}

	got := captureStdout(t, func() {
		if err := printResponse(resp); err != nil {
			t.Fatalf("printResponse error: %v", err)
		}
	})

	if !bytes.Contains([]byte(got), []byte(`"currentSize": 3`)) {
		t.Fatalf("expected currentSize in output, got: %s", got)
	}
	if !bytes.Contains([]byte(got), []byte(`"desiredSize": 4`)) {
		t.Fatalf("expected desiredSize in output, got: %s", got)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	defer func() {
		os.Stdout = oldStdout
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return string(out)
}

func TestMultiFlag(t *testing.T) {
	var m multiFlag

	if m.String() != "" {
		t.Fatalf("expected empty string, got: %q", m.String())
	}

	if err := m.Set("val1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m.String() != "val1" {
		t.Fatalf("expected \"val1\", got: %q", m.String())
	}

	if err := m.Set("val2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m.String() != "val1,val2" {
		t.Fatalf("expected \"val1,val2\", got: %q", m.String())
	}

	if len(m) != 2 {
		t.Fatalf("expected length 2, got: %d", len(m))
	}
	if m[0] != "val1" || m[1] != "val2" {
		t.Fatalf("expected [val1 val2], got: %v", m)
	}
}
