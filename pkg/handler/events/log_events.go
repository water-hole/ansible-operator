package events

import (
	"github.com/sirupsen/logrus"
	"github.com/water-hole/ansible-operator/pkg/runner/eventapi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// LogLevel - Levelt for the logging to take place.
type LogLevel int

const (
	// Everything - log every event.
	Everything LogLevel = iota
	// Tasks - only log the high level tasks.
	Tasks
	// Nothing -  this will log nothing.
	Nothing
)

// EventHandler - knows how to handle job events.
type EventHandler interface {
	Handle(*unstructured.Unstructured, eventapi.JobEvent)
}

type loggingEventHandler struct {
	LogLevel LogLevel
}

func (l loggingEventHandler) Handle(u *unstructured.Unstructured, e eventapi.JobEvent) {
	log := logrus.WithFields(logrus.Fields{
		"component":  "logging_event_handler",
		"name":       u.GetName(),
		"namespace":  u.GetNamespace(),
		"gvk":        u.GroupVersionKind().String(),
		"event_type": e.Event,
	})
	switch l.LogLevel {
	case Everything:
		log.Infof("event: %#v", u)
	case Tasks:
		if t, ok := e.EventData["task"]; ok {
			log.WithField("task", t).Infof("%v", u)
		}
	}
}

// NewLoggingEventHandler - Creates a Logging Event Handler to log events.
func NewLoggingEventHandler(l LogLevel) EventHandler {
	return loggingEventHandler{
		LogLevel: l,
	}
}
