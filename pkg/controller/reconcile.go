package controller

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/water-hole/ansible-operator/pkg/events"
	"github.com/water-hole/ansible-operator/pkg/proxy/kubeconfig"
	"github.com/water-hole/ansible-operator/pkg/runner"
	"github.com/water-hole/ansible-operator/pkg/runner/eventapi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ReconcileAnsibleOperator - object to reconcile runner requests
type ReconcileAnsibleOperator struct {
	GVK           schema.GroupVersionKind
	Runner        runner.Runner
	Client        client.Client
	EventHandlers []events.EventHandler
}

// Reconcile - handle the event.
func (r *ReconcileAnsibleOperator) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(r.GVK)
	r.Client.Get(context.TODO(), request.NamespacedName, u)
	s := u.Object["spec"]
	_, ok := s.(map[string]interface{})
	if !ok {
		logrus.Warnf("spec was not found")
		u.Object["spec"] = map[string]interface{}{}
		r.Client.Update(context.TODO(), u)
		return reconcile.Result{Requeue: true}, nil
	}
	ownerRef := metav1.OwnerReference{
		APIVersion: u.GetAPIVersion(),
		Kind:       u.GetKind(),
		Name:       u.GetName(),
		UID:        u.GetUID(),
	}

	kc, err := kubeconfig.Create(ownerRef, "http://localhost:8888", u.GetNamespace())
	if err != nil {
		return reconcile.Result{}, err
	}
	defer os.Remove(kc.Name())
	eventChan, err := r.Runner.Run(u, kc.Name())
	if err != nil {
		return reconcile.Result{}, err
	}

	// iterate events from ansible, looking for the final one
	statusEvent := eventapi.StatusJobEvent{}
	for event := range eventChan {
		for _, eHandler := range r.EventHandlers {
			go eHandler.Handle(u, event)
		}
		if event.Event == "playbook_on_stats" {
			// convert to StatusJobEvent; would love a better way to do this
			data, err := json.Marshal(event)
			if err != nil {
				return reconcile.Result{}, err
			}
			err = json.Unmarshal(data, &statusEvent)
			if err != nil {
				return reconcile.Result{}, err
			}
		}
	}
	if statusEvent.Event == "" {
		err := errors.New("did not receive playbook_on_stats event")
		logrus.Error(err.Error())
		return reconcile.Result{}, err
	}

	statusMap, ok := u.Object["status"].(map[string]interface{})
	if !ok {
		u.Object["status"] = ResourceStatus{
			Status: NewStatusFromStatusJobEvent(statusEvent),
		}
		r.Client.Update(context.TODO(), u)
		logrus.Infof("adding status for the first time")
		return reconcile.Result{}, nil
	}
	// Need to conver the map[string]interface into a resource status.
	if update, status := UpdateResourceStatus(statusMap, statusEvent); update {
		u.Object["status"] = status
		r.Client.Update(context.TODO(), u)
		return reconcile.Result{}, nil
	}
	return reconcile.Result{}, nil
}
