package stub

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
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
	kubeconfig, err := createKubeConfig(u)
	if err != nil {
		// TODO Do something
	}
	s := u.Object["spec"]
	spec, ok := s.(map[string]interface{})
	if !ok {
		u.Object["spec"] = map[string]interface{}{}
		sdk.Update(u)
		logrus.Warnf("spec is not a map[string]interface{} - %#v", s)
		return nil
	}

	return runPlaybook(p, spec, kubeconfig)
}

func createKubeConfig(object *unstructured.Unstructured) (string, error) {
	file, err := ioutil.TempFile("", "kubeconfig")
	if err != nil {
		return "", err
	}

	tmpl := `---
apiVersion: v1
kind: Config
clusters:
- cluster:
    insecure-skip-tls-verify: true
    server: http://{{.Credentials}}@{{.ProxyServer}}
  name: proxy-server
contexts:
- context:
    cluster: proxy-server
    user: admin/proxy-server
  name: {{.Namespace}}/proxy-server
current-context: {{.Namespace}}/proxy-server
preferences: {}
users:
- name: admin/proxy-server
`
	var parsed bytes.Buffer
	credentials, _ := json.Marshal(map[string]string{
		"apiVersion": object.GetAPIVersion(),
		"kind":       object.GetKind(),
		"name":       object.GetName(),
		"uid":        string(object.GetUID()),
	})

	t := template.Must(template.New("kubeconfig").Parse(tmpl))
	t.Execute(&parsed, struct {
		Credentials string
		ProxyServer string
		Namespace   string
	}{
		Credentials: base64.URLEncoding.EncodeToString([]byte(credentials)),
		ProxyServer: "127.0.0.1:8001",
		Namespace:   object.GetNamespace(),
	})

	if _, err := file.WriteString(parsed.String()); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return file.Name(), nil

}

func runPlaybook(path string, parameters map[string]interface{}, kubeconfig string) error {
	b, err := json.Marshal(parameters)
	if err != nil {
		return err
	}
	dc := exec.Command("ansible-playbook", path, "-vv", "--extra-vars", string(b))
	dc.Env = append(os.Environ(),
		fmt.Sprintf("K8s_AUTH_KUBECONFIG=%s", kubeconfig),
	)
	dc.Stdout = os.Stdout
	dc.Stderr = os.Stderr
	return dc.Run()
}
