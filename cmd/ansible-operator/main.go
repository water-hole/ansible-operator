package main

import (
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"time"

	"github.com/operator-framework/operator-sdk/pkg/k8sclient"
	sdk "github.com/operator-framework/operator-sdk/pkg/sdk"
	k8sutil "github.com/operator-framework/operator-sdk/pkg/util/k8sutil"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	handler "github.com/water-hole/ansible-operator/pkg/handler"
	proxy "github.com/water-hole/ansible-operator/pkg/proxy"
	"github.com/water-hole/ansible-operator/pkg/runner"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	"github.com/sirupsen/logrus"
)

func printVersion() {
	logrus.Infof("Go Version: %s", runtime.Version())
	logrus.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	logrus.Infof("operator-sdk Version: %v", sdkVersion.Version)
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

func registerGVK(gvk schema.GroupVersionKind) {
	schemeBuilder := k8sruntime.NewSchemeBuilder(func(s *k8sruntime.Scheme) error {
		s.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		return nil
	})
	k8sutil.AddToSDKScheme(schemeBuilder.AddToScheme)
}

// Used to set the decoder to just deserialize instead of decode.
// This is only need as a work around for now and when the SDK is based on
// controller-runtime we expect this to be fixed.
// This issue https://github.com/operator-framework/operator-sdk/issues/382
// is tracking this process.
func decoder(gv schema.GroupVersion, codecs serializer.CodecFactory) k8sruntime.Decoder {
	return codecs.UniversalDeserializer()
}

func runSDK(done chan error) {
	// setting the utility decoder function.
	k8sutil.SetDecoderFunc(decoder)
	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		logrus.Error("Failed to get watch namespace")
		done <- err
		return
	}
	resyncPeriod := 60
	configs, err := runner.NewFromConfig("/opt/ansible/config.yaml")
	if err != nil {
		logrus.Error("Failed to get configs")
		done <- err
		return
	}
	rand.Seed(time.Now().Unix())

	for gvk, _ := range configs {
		logrus.Infof("Watching %s/%v, %s, %s, %d", gvk.Group, gvk.Version, gvk.Kind, namespace, resyncPeriod)
		registerGVK(gvk)
		sdk.Watch(fmt.Sprintf("%v/%v", gvk.Group, gvk.Version), gvk.Kind, namespace, resyncPeriod)

	}
	sdk.Handle(handler.New(configs))
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
