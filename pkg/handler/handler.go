package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/sirupsen/logrus"
	"github.com/water-hole/ansible-operator/pkg/proxy/kubeconfig"
	"github.com/water-hole/ansible-operator/pkg/runner"
	"github.com/water-hole/ansible-operator/pkg/runner/eventapi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// EventHandler used to handle a runner event. This will be called if a
// mapping is found for the GVK of the event. This will be used as an override
// for the default handle implementation. To set this for the handler, set the
// options Handle with your implementation of this interface, and it will be
// used.
type EventHandler interface {
	Handle(context.Context, sdk.Event, runner.Runner) error
}

// EventHandlerFunc is a adapter to use functions as handlers.
type EventHandlerFunc func(context.Context, sdk.Event, runner.Runner) error

// Handle calls f(ctx, event, run)
func (f EventHandlerFunc) Handle(ctx context.Context, event sdk.Event, run runner.Runner) error {
	return f(ctx, event, run)
}

func defaultHandle(ctx context.Context, event sdk.Event, run runner.Runner) error {
	u, ok := event.Object.(*unstructured.Unstructured)
	if !ok {
		logrus.Warnf("object was not unstructured - %#v", event.Object)
		return nil
	}
	//Default spec to empty map.
	s := u.Object["spec"]
	_, ok = s.(map[string]interface{})
	if !ok {
		logrus.Warnf("spec was not found")
		u.Object["spec"] = map[string]interface{}{}
		sdk.Update(u)
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
	eventChan, err := run.Run(u, kc.Name())
	if err != nil {
		return err
	}

	// iterate events from ansible, looking for the final one
	statusEvent := eventapi.StatusJobEvent{}
	for event := range eventChan {
		if event.Event == "playbook_on_stats" {
			// convert to StatusJobEvent; would love a better way to do this
			data, err := json.Marshal(event)
			if err != nil {
				return err
			}
			err = json.Unmarshal(data, &statusEvent)
			if err != nil {
				return err
			}
		}
	}
	if statusEvent.Event == "" {
		err := errors.New("did not receive playbook_on_stats event")
		logrus.Error(err.Error())
		return err
	}

	statusMap, ok := u.Object["status"].(map[string]interface{})
	if !ok {
		u.Object["status"] = ResourceStatus{
			Status: NewStatusFromStatusJobEvent(statusEvent),
		}
		sdk.Update(u)
		logrus.Infof("adding status for the first time")
		return nil
	}
	// Need to conver the map[string]interface into a resource status.
	if update, status := UpdateResourceStatus(statusMap, statusEvent); update {
		u.Object["status"] = status
		sdk.Update(u)
		return nil
	}
	return nil
}

// Options will be used to tell the new ansible handler how to behave. You have
// the ability to set the Interface that will be used to handle the sdk.Event.
// The GVKToRunner map must be passed in and must have at least a single
// mapping.
type Options struct {
	Handle      EventHandler
	GVKToRunner map[schema.GroupVersionKind]runner.Runner
}

// New will create a ansible handler to be used by the sdk. New will create a
// sdk.Handler that will manage the GVKToRunner map to send the correct runner
// to the handle Method of the Interface interface. A default handle will be
// used if one is not set in the options.
func New(options Options) (sdk.Handler, error) {
	if len(options.GVKToRunner) == 0 {
		return nil, fmt.Errorf("options must contain a gvk runner mapping")
	}
	var handle EventHandler = EventHandlerFunc(defaultHandle)
	if options.Handle != nil {
		handle = options.Handle
	}
	return &handler{crdToPlaybook: options.GVKToRunner, handle: handle}, nil
}

type handler struct {
	crdToPlaybook map[schema.GroupVersionKind]runner.Runner
	handle        EventHandler
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
