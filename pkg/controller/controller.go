package controller

import (
	"fmt"
	"log"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/water-hole/ansible-operator/pkg/events"
	"github.com/water-hole/ansible-operator/pkg/runner"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	crthandler "sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func NewController(mrg manager.Manager, gvk schema.GroupVersionKind, namespace string, runner runner.Runner) {
	logrus.Infof("Watching %s/%v, %s, %s", gvk.Group, gvk.Version, gvk.Kind, namespace)
	h := &ReconcileAnsibleOperator{
		Client: mrg.GetClient(),
		GVK:    gvk,
		Runner: runner,
		EventHandlers: []events.EventHandler{
			events.NewLoggingEventHandler(events.Tasks),
		},
	}
	mrg.GetScheme().AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
	metav1.AddToGroupVersion(mrg.GetScheme(), schema.GroupVersion{
		Group:   gvk.Group,
		Version: gvk.Version,
	})
	//Create new controllers for each gvk.
	c, err := controller.New(fmt.Sprintf("%v-controller", strings.ToLower(gvk.Kind)), mrg, controller.Options{
		Reconciler: h,
	})
	if err != nil {
		log.Fatal(err)
	}
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(gvk)
	if err := c.Watch(&source.Kind{Type: u}, &crthandler.EnqueueRequestForObject{}); err != nil {
		log.Fatal(err)
	}
}
