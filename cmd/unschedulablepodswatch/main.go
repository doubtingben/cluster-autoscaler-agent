package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

type config struct {
	kubeconfig string
	context    string
	namespace  string
}

type unschedulableSnapshot struct {
	Namespace         string            `json:"namespace"`
	Name              string            `json:"name"`
	UID               types.UID         `json:"uid"`
	SchedulerName     string            `json:"schedulerName,omitempty"`
	NominatedNodeName string            `json:"nominatedNodeName,omitempty"`
	Reason            string            `json:"reason"`
	Message           string            `json:"message,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	CreatedAt         time.Time         `json:"createdAt"`
}

type podEvent struct {
	EventType string                `json:"eventType"`
	Pod       unschedulableSnapshot `json:"pod"`
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.kubeconfig, "kubeconfig", "", "path to kubeconfig file; defaults to in-cluster or standard kubeconfig loading")
	flag.StringVar(&cfg.context, "context", "", "kubeconfig context override")
	flag.StringVar(&cfg.namespace, "namespace", metav1.NamespaceAll, "namespace to watch; defaults to all namespaces")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: cfg.kubeconfig},
		&clientcmd.ConfigOverrides{CurrentContext: cfg.context},
	).ClientConfig()
	if err != nil {
		fatal(err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		fatal(err)
	}

	logf("connecting to Kubernetes API")

	factory := informers.NewSharedInformerFactoryWithOptions(
		clientset,
		0,
		informers.WithNamespace(cfg.namespace),
		informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			opts.FieldSelector = fields.OneTermEqualSelector("status.phase", string(corev1.PodPending)).String()
		}),
	)
	informer := factory.Core().V1().Pods().Informer()

	state := map[types.UID]unschedulableSnapshot{}
	_, err = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			pod, ok := obj.(*corev1.Pod)
			if !ok {
				return
			}
			snap, ok := unschedulableSnapshotForPod(pod)
			if !ok {
				return
			}
			state[pod.UID] = snap
			emit("ADDED", snap)
		},
		UpdateFunc: func(oldObj, newObj any) {
			pod, ok := newObj.(*corev1.Pod)
			if !ok {
				return
			}
			snap, isUnschedulable := unschedulableSnapshotForPod(pod)
			prev, hadPrev := state[pod.UID]

			switch {
			case isUnschedulable && !hadPrev:
				state[pod.UID] = snap
				emit("ADDED", snap)
			case isUnschedulable && hadPrev && !snapEqual(prev, snap):
				state[pod.UID] = snap
				emit("MODIFIED", snap)
			case !isUnschedulable && hadPrev:
				delete(state, pod.UID)
				emit("DELETED", prev)
			}
		},
		DeleteFunc: func(obj any) {
			pod, ok := extractPod(obj)
			if !ok {
				return
			}
			prev, hadPrev := state[pod.UID]
			if !hadPrev {
				return
			}
			delete(state, pod.UID)
			emit("DELETED", prev)
		},
	})
	if err != nil {
		fatal(err)
	}

	logf("starting unschedulable pod watch for namespace=%q", watchNamespaceLabel(cfg.namespace))
	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		fatal(fmt.Errorf("timed out waiting for pod informer cache sync"))
	}
	logf("watch connected and cache synced")

	<-ctx.Done()
	logf("stopping unschedulable pod watch")
	logf("watch disconnected and cleanup complete")
}

func unschedulableSnapshotForPod(pod *corev1.Pod) (unschedulableSnapshot, bool) {
	if pod == nil || pod.Spec.NodeName != "" || pod.Status.Phase != corev1.PodPending {
		return unschedulableSnapshot{}, false
	}

	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse && cond.Reason == corev1.PodReasonUnschedulable {
			return unschedulableSnapshot{
				Namespace:         pod.Namespace,
				Name:              pod.Name,
				UID:               pod.UID,
				SchedulerName:     pod.Spec.SchedulerName,
				NominatedNodeName: pod.Status.NominatedNodeName,
				Reason:            cond.Reason,
				Message:           cond.Message,
				Labels:            pod.Labels,
				CreatedAt:         pod.CreationTimestamp.Time.UTC(),
			}, true
		}
	}

	return unschedulableSnapshot{}, false
}

func snapEqual(a, b unschedulableSnapshot) bool {
	if a.Namespace != b.Namespace ||
		a.Name != b.Name ||
		a.UID != b.UID ||
		a.SchedulerName != b.SchedulerName ||
		a.NominatedNodeName != b.NominatedNodeName ||
		a.Reason != b.Reason ||
		a.Message != b.Message ||
		!a.CreatedAt.Equal(b.CreatedAt) {
		return false
	}
	return mapsEqual(a.Labels, b.Labels)
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func extractPod(obj any) (*corev1.Pod, bool) {
	if pod, ok := obj.(*corev1.Pod); ok {
		return pod, true
	}
	tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
	if !ok {
		return nil, false
	}
	pod, ok := tombstone.Obj.(*corev1.Pod)
	return pod, ok
}

func emit(eventType string, snap unschedulableSnapshot) {
	out, err := json.Marshal(podEvent{
		EventType: eventType,
		Pod:       snap,
	})
	if err != nil {
		fatal(err)
	}
	fmt.Println(string(out))
}

func watchNamespaceLabel(namespace string) string {
	if namespace == metav1.NamespaceAll {
		return "all"
	}
	return namespace
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
