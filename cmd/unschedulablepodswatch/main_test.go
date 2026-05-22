package main

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestUnschedulableSnapshotForPod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         "default",
			Name:              "p1",
			UID:               types.UID("pod-1"),
			CreationTimestamp: metav1.NewTime(time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)),
			Labels:            map[string]string{"app": "demo"},
		},
		Spec: corev1.PodSpec{
			SchedulerName: "default-scheduler",
		},
		Status: corev1.PodStatus{
			Phase:             corev1.PodPending,
			NominatedNodeName: "node-a",
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  corev1.PodReasonUnschedulable,
					Message: "0/3 nodes are available",
				},
			},
		},
	}

	got, ok := unschedulableSnapshotForPod(pod)
	if !ok {
		t.Fatalf("expected pod to be unschedulable")
	}
	if got.Name != "p1" || got.Namespace != "default" {
		t.Fatalf("unexpected pod identity: %+v", got)
	}
	if got.Reason != corev1.PodReasonUnschedulable {
		t.Fatalf("unexpected reason: %+v", got)
	}
}

func TestUnschedulableSnapshotForPodIgnoresScheduledOrNonMatchingPods(t *testing.T) {
	tests := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "scheduled"},
			Spec:       corev1.PodSpec{NodeName: "node-1"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "plain-pending"},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "other-reason"},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				Conditions: []corev1.PodCondition{{
					Type:   corev1.PodScheduled,
					Status: corev1.ConditionFalse,
					Reason: "SchedulerNotReady",
				}},
			},
		},
	}

	for _, tt := range tests {
		if _, ok := unschedulableSnapshotForPod(&tt); ok {
			t.Fatalf("expected pod %q to be ignored", tt.Name)
		}
	}
}

func TestMapsEqual(t *testing.T) {
	tests := []struct {
		name string
		a    map[string]string
		b    map[string]string
		want bool
	}{
		{
			name: "both nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "one nil, one empty",
			a:    nil,
			b:    map[string]string{},
			want: true,
		},
		{
			name: "both empty",
			a:    map[string]string{},
			b:    map[string]string{},
			want: true,
		},
		{
			name: "different lengths",
			a:    map[string]string{"k1": "v1"},
			b:    map[string]string{},
			want: false,
		},
		{
			name: "same lengths, different keys",
			a:    map[string]string{"k1": "v1"},
			b:    map[string]string{"k2": "v1"},
			want: false,
		},
		{
			name: "same lengths, same keys, different values",
			a:    map[string]string{"k1": "v1"},
			b:    map[string]string{"k1": "v2"},
			want: false,
		},
		{
			name: "equal maps",
			a:    map[string]string{"k1": "v1", "k2": "v2"},
			b:    map[string]string{"k1": "v1", "k2": "v2"},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapsEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("mapsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}
