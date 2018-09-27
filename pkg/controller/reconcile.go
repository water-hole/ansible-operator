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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// AnsibleOperatorReconciler - object to reconcile runner requests
type AnsibleOperatorReconciler struct {
	GVK           schema.GroupVersionKind
	Runner        runner.Runner
	Client        client.Client
	EventHandlers []events.EventHandler
}

// Reconcile - handle the event.
func (r *AnsibleOperatorReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(r.GVK)
	err := r.Client.Get(context.TODO(), request.NamespacedName, u)
	if apierrors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	deleted := u.GetDeletionTimestamp() != nil
	finalizer, finalizerExists := r.Runner.GetFinalizer()
	pendingFinalizers := u.GetFinalizers()
	// If the resource is being deleted we don't want to add the finalizer again
	if finalizerExists && !deleted && !contains(pendingFinalizers, finalizer) {
		logrus.Debugf("Adding finalizer %s to resource", finalizer)
		finalizers := append(pendingFinalizers, finalizer)
		u.SetFinalizers(finalizers)
		err := r.Client.Update(context.TODO(), u)
		return reconcile.Result{}, err
	}
	if !contains(pendingFinalizers, finalizer) && deleted {
		logrus.Info("Resource is terminated, skipping reconcilation")
		return reconcile.Result{}, nil
	}

	spec := u.Object["spec"]
	_, ok := spec.(map[string]interface{})
	if !ok {
		logrus.Warnf("spec was not found")
		u.Object["spec"] = map[string]interface{}{}
		r.Client.Update(context.TODO(), u)
		return reconcile.Result{Requeue: true}, nil
	}
	status := u.Object["status"]
	_, ok = status.(map[string]interface{})
	if !ok {
		logrus.Warnf("status was not found")
		u.Object["status"] = map[string]interface{}{}
		r.Client.Update(context.TODO(), u)
		return reconcile.Result{Requeue: true}, nil
	}

	// If status is an empty map we can assume CR was just created
	if len(u.Object["status"].(map[string]interface{})) == 0 {
		logrus.Debugf("Setting phase status to Creating")
		u.Object["status"] = ResourceStatus{
			Status: Status{
				Phase: "Creating",
			},
		}
		r.Client.Update(context.TODO(), u)
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

	// We only want to update the CustomResource once, so we'll track changes and do it at the end
	var needsUpdate bool
	runSuccessful := true
	for _, count := range statusEvent.EventData.Failures {
		if count > 0 {
			runSuccessful = false
			break
		}
	}
	// The finalizer has run successfully, time to remove it
	if deleted && finalizerExists && runSuccessful {
		finalizers := []string{}
		for _, pendingFinalizer := range pendingFinalizers {
			if pendingFinalizer != finalizer {
				finalizers = append(finalizers, pendingFinalizer)
			}
		}
		u.SetFinalizers(finalizers)
		needsUpdate = true
	}

	statusMap, ok := u.Object["status"].(map[string]interface{})
	if !ok {
		u.Object["status"] = ResourceStatus{
			Status: NewStatusFromStatusJobEvent(statusEvent),
		}
		logrus.Infof("adding status for the first time")
		needsUpdate = true
	} else {
		// Need to conver the map[string]interface into a resource status.
		if update, status := UpdateResourceStatus(statusMap, statusEvent); update {
			u.Object["status"] = status
			needsUpdate = true
		}
	}
	if needsUpdate {
		err = r.Client.Update(context.TODO(), u)
	}
	if !runSuccessful {
		return reconcile.Result{Requeue: true}, err
	}
	return reconcile.Result{}, err
}

func contains(l []string, s string) bool {
	for _, elem := range l {
		if elem == s {
			return true
		}
	}
	return false
}
