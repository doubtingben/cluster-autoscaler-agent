package main

import (
	"context"
	"testing"
)

func TestBuildCall(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantMethod string
		wantBody   string
		wantErr    bool
	}{
		{name: "list", args: []string{"list-nodegroups"}, wantMethod: "NodeGroups", wantBody: `{}`},
		{name: "template", args: []string{"template", "--nodegroup", "ng-1"}, wantMethod: "NodeGroupTemplateNodeInfo", wantBody: `{"id":"ng-1"}`},
		{name: "node-info", args: []string{"node-info", "--nodegroup", "ng-1"}, wantMethod: "NodeGroupNodes", wantBody: `{"id":"ng-1"}`},
		{name: "scale-up", args: []string{"scale-up", "--nodegroup", "ng-1", "--delta", "2"}, wantMethod: "IncreaseSize", wantBody: `{"delta":2,"id":"ng-1"}`},
		{name: "scale-down", args: []string{"scale-down", "--nodegroup", "ng-1", "--delta", "2"}, wantMethod: "DecreaseTargetSize", wantBody: `{"delta":-2,"id":"ng-1"}`},
		{name: "invalid", args: []string{"oops"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, gotMethod, gotBody, err := buildCall(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildCall error: %v", err)
			}
			if gotMethod != tt.wantMethod {
				t.Fatalf("method = %q, want %q", gotMethod, tt.wantMethod)
			}
			if gotBody != tt.wantBody {
				t.Fatalf("body = %q, want %q", gotBody, tt.wantBody)
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
