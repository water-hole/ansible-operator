package controller

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// ReconcileLoop - new loop
type ReconcileLoop struct {
	Source   chan event.GenericEvent
	Stop     <-chan struct{}
	GVK      schema.GroupVersionKind
	Interval time.Duration
	Client   client.Client
}

// NewReconcileLoop - loop for a GVK.
func NewReconcileLoop(interval time.Duration, gvk schema.GroupVersionKind, c client.Client) ReconcileLoop {
	s := make(chan event.GenericEvent, 1025)
	return ReconcileLoop{
		Source:   s,
		GVK:      gvk,
		Interval: interval,
		Client:   c,
	}
}

// Start - start the reconcile loop
func (r *ReconcileLoop) Start() {
	go func() {
		ticker := time.NewTicker(r.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// List all object for the GVK
				ul := &unstructured.UnstructuredList{}
				ul.SetGroupVersionKind(r.GVK)
				err := r.Client.List(context.Background(), nil, ul)
				if err != nil {
					logrus.Warningf("unable to list resources for GV: %v during reconcilation", r.GVK)
					continue
				}
				for _, u := range ul.Items {
					e := event.GenericEvent{
						Meta:   &u,
						Object: &u,
					}
					r.Source <- e
				}
			case <-r.Stop:
				return
			}
		}
	}()
}
