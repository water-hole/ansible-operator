package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"runtime"
	"time"

	proxy "github.com/automationbroker/ansible-operator/pkg/proxy"
	"github.com/automationbroker/ansible-operator/pkg/runner"
	stub "github.com/automationbroker/ansible-operator/pkg/stub"
	"github.com/operator-framework/operator-sdk/pkg/k8sclient"
	sdk "github.com/operator-framework/operator-sdk/pkg/sdk"
	k8sutil "github.com/operator-framework/operator-sdk/pkg/util/k8sutil"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	yaml "gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/sirupsen/logrus"
)

func printVersion() {
	logrus.Infof("Go Version: %s", runtime.Version())
	logrus.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	logrus.Infof("operator-sdk Version: %v", sdkVersion.Version)
}

type config struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	Group   string `yaml:"group"`
	Kind    string `yaml:"kind"`
	// Command to execute.
	//Command string `yaml:"command"`
	// Path that will be passed to the command.
	Path string `yaml:"path"`
}

func main() {
	printVersion()
	done := make(chan error)

	// start the proxy
	go runProxy("localhost", 8888, done)

	// start the operator
	go runSDK(done)

	// wait for either to finish
	err := <-done
	if err == nil {
		logrus.Info("Exiting")
	} else {
		logrus.Fatal(err.Error())
	}
}

// readConfig reads the operator's config file at /opt/ansible/config.yaml
func readConfig() ([]config, error) {
	b, err := ioutil.ReadFile("/opt/ansible/config.yaml")
	if err != nil {
		logrus.Fatalf("failed to get config file %v", err)
	}
	c := []config{}
	err = yaml.Unmarshal(b, &c)
	if err != nil {
		logrus.Fatalf("failed to unmarshal config %v", err)
	}
	return c, nil
}

func registerGVK(gvk schema.GroupVersionKind) {
	schemeBuilder := k8sruntime.NewSchemeBuilder(func(s *k8sruntime.Scheme) error {
		s.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		return nil
	})
	k8sutil.AddToSDKScheme(schemeBuilder.AddToScheme)
}

func runSDK(done chan error) {
	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		logrus.Error("Failed to get watch namespace")
		done <- err
		return
	}
	resyncPeriod := 60
	configs, err := readConfig()
	if err != nil {
		logrus.Error("Failed to get configs")
		done <- err
		return
	}
	rand.Seed(time.Now().Unix())

	m := map[schema.GroupVersionKind]runner.Runner{}

	for _, c := range configs {
		logrus.Infof("Watching %s/%v, %s, %s, %d path: %v", c.Group, c.Version, c.Kind, namespace, resyncPeriod, c.Path)
		s := schema.GroupVersionKind{
			Group:   c.Group,
			Version: c.Version,
			Kind:    c.Kind,
		}
		registerGVK(s)
		m[s] = &runner.Playbook{
			Path: c.Path,
			GVK:  s,
		}
		sdk.Watch(fmt.Sprintf("%v/%v", c.Group, c.Version), c.Kind, namespace, resyncPeriod)

	}
	sdk.Handle(stub.NewHandler(m))
	sdk.Run(context.TODO())
	done <- nil
}

func runProxy(address string, port int, done chan error) {
	clientConfig := k8sclient.GetKubeConfig()

	server, err := proxy.NewServer("/", clientConfig)
	if err != nil {
		done <- err
		return
	}
	l, err := server.Listen(address, port)
	if err != nil {
		done <- err
		return
	}
	logrus.Infof("Starting to serve on %s\n", l.Addr().String())
	done <- server.ServeOnListener(l)
}
