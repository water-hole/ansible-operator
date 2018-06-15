package stub

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"

	"github.com/operator-framework/operator-sdk/pkg/sdk"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func NewHandler(m map[schema.GroupVersionKind]string) sdk.Handler {
	return &Handler{crdToPlaybook: m}
}

type Handler struct {
	crdToPlaybook map[schema.GroupVersionKind]string
	// Fill me
}

func (h *Handler) Handle(ctx context.Context, event sdk.Event) error {
	p, ok := h.crdToPlaybook[event.Object.GetObjectKind().GroupVersionKind()]
	if !ok {
		logrus.Warnf("unable to find playbook mapping for gvk: %v", event.Object.GetObjectKind().GroupVersionKind())
		return nil
	}
	u, ok := event.Object.(*unstructured.Unstructured)
	if !ok {
		logrus.Warnf("object was not unstructured - %#v", event.Object)
		return nil
	}
	s := u.Object["spec"]
	spec, ok := s.(map[string]interface{})
	if !ok {
		u.Object["spec"] = map[string]interface{}{}
		sdk.Update(u)
		logrus.Warnf("spec is not a map[string]interface{} - %#v", s)
		return nil
	}

	return runPlaybook(p, spec)
}

func runPlaybook(path string, parameters map[string]interface{}) error {
	b, err := json.Marshal(parameters)
	if err != nil {
		return err
	}
	dc := exec.Command("ansible-playbook", path, "--extra-vars", string(b))
	dc.Stdout = os.Stdout
	dc.Stderr = os.Stderr
	return dc.Run()
}
