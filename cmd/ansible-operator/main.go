package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"runtime"

	proxy "github.com/automationbroker/ansible-operator/pkg/proxy"
	stub "github.com/automationbroker/ansible-operator/pkg/stub"
	sdk "github.com/operator-framework/operator-sdk/pkg/sdk"
	k8sutil "github.com/operator-framework/operator-sdk/pkg/util/k8sutil"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	yaml "gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

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
	go RunProxy("127.0.0.1", 8001)
	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		logrus.Fatalf("Failed to get watch namespace: %v", err)
	}
	resyncPeriod := 60
	configs, err := readConfig()
	if err != nil {
		logrus.Fatalf("Failed to get configs: %v", err)
	}

	m := map[schema.GroupVersionKind]string{}

	for _, c := range configs {
		logrus.Infof("Watching %s/%v, %s, %s, %d path: %v", c.Group, c.Version, c.Kind, namespace, resyncPeriod, c.Path)
		s := schema.GroupVersionKind{
			Group:   c.Group,
			Version: c.Version,
			Kind:    c.Kind,
		}
		registerGVK(s)
		m[s] = c.Path
		sdk.Watch(fmt.Sprintf("%v/%v", c.Group, c.Version), c.Kind, namespace, resyncPeriod)

	}
	sdk.Handle(stub.NewHandler(m))
	sdk.Run(context.TODO())
}

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

func RunProxy(address string, port int) error {
	clientConfig, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	server, err := proxy.NewServer("", nil, clientConfig)
	if err != nil {
		return err
	}
	l, err := server.Listen(address, port)
	if err != nil {
		return err
	}
	logrus.Infof("Starting to serve on %s\n", l.Addr().String())
	logrus.Fatal(server.ServeOnListener(l))
	return nil
}
