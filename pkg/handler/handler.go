package handler

import (
	"context"
	"fmt"
	"os"

	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/sirupsen/logrus"
	"github.com/water-hole/ansible-operator/pkg/proxy/kubeconfig"
	"github.com/water-hole/ansible-operator/pkg/runner"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Interface used to handle a runner event. This will be called if a mapping is found
// for the GVK of the event.
type Interface interface {
	Handle(context.Context, sdk.Event, runner.Runner) error
}

// InterfaceFunc is a adapter to use functions as handlers.
type InterfaceFunc func(context.Context, sdk.Event, runner.Runner) error

// Handle calls f(ctx, event, run)
func (f InterfaceFunc) Handle(ctx context.Context, event sdk.Event, run runner.Runner) error {
	return f(ctx, event, run)
}

func defaultHandle(ctx context.Context, event sdk.Event, run runner.Runner) error {
	u, ok := event.Object.(*unstructured.Unstructured)
	if !ok {
		logrus.Warnf("object was not unstructured - %#v", event.Object)
		return nil
	}
	ownerRef := metav1.OwnerReference{
		APIVersion: u.GetAPIVersion(),
		Kind:       u.GetKind(),
		Name:       u.GetName(),
		UID:        u.GetUID(),
	}

	kc, err := kubeconfig.Create(ownerRef, "http://localhost:8888", u.GetNamespace())
	if err != nil {
		return err
	}
	defer os.Remove(kc.Name())

	s := u.Object["spec"]
	spec, ok := s.(map[string]interface{})
	if !ok {
		u.Object["spec"] = map[string]interface{}{}
		sdk.Update(u)
		logrus.Warnf("spec is not a map[string]interface{} - %#v", s)
		return nil
	}
	statusEvent, err := run.Run(spec, u.GetName(), u.GetNamespace(), kc.Name())
	if err != nil {
		return err
	}
	statusMap, ok := u.Object["status"].(map[string]interface{})
	if !ok {
		u.Object["status"] = runner.ResourceStatus{
			Status: runner.NewStatusFromStatusJobEvent(statusEvent),
		}
		sdk.Update(u)
		logrus.Infof("adding status for the first time")
		return nil
	}
	// Need to conver the map[string]interface into a resource status.
	if update, status := runner.UpdateResourceStatus(statusMap, statusEvent); update {
		u.Object["status"] = status
		sdk.Update(u)
		return nil
	}
	return nil
}

// Options will be used to tell the new ansible handler how to behave.
type Options struct {
	Handle      Interface
	GVKToRunner map[schema.GroupVersionKind]runner.Runner
}

// New will create a ansible handler to be used by the sdk.
func New(options Options) (sdk.Handler, error) {
	if len(options.GVKToRunner) == 0 {
		return nil, fmt.Errorf("options must contain a gvk runner mapping")
	}
	var handle Interface = InterfaceFunc(defaultHandle)
	if options.Handle != nil {
		handle = options.Handle
	}
	return &handler{crdToPlaybook: options.GVKToRunner, handle: handle}, nil
}

type handler struct {
	crdToPlaybook map[schema.GroupVersionKind]runner.Runner
	handle        Interface
}

// Handle conform to the sdk.Handle interface.
func (h *handler) Handle(ctx context.Context, event sdk.Event) error {
	p, ok := h.crdToPlaybook[event.Object.GetObjectKind().GroupVersionKind()]
	if !ok {
		logrus.Warnf("unable to find playbook mapping for gvk: %v", event.Object.GetObjectKind().GroupVersionKind())
		return nil
	}
	return h.handle.Handle(ctx, event, p)
}
