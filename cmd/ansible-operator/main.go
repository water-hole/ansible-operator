package main

import (
	"context"
	"fmt"
	"math/rand"
	"runtime"
	"time"

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
	proxy.RunProxy(done, proxy.Options{
		Address: "localhost",
		Port:    8888,
	})

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
	watches, err := runner.NewFromWatches("/opt/ansible/watches.yaml")
	if err != nil {
		logrus.Error("Failed to get watches")
		done <- err
		return
	}
	rand.Seed(time.Now().Unix())

	for gvk := range watches {
		logrus.Infof("Watching %s/%v, %s, %s, %d", gvk.Group, gvk.Version, gvk.Kind, namespace, resyncPeriod)
		registerGVK(gvk)
		sdk.Watch(fmt.Sprintf("%v/%v", gvk.Group, gvk.Version), gvk.Kind, namespace, resyncPeriod)

	}
	h, err := handler.New(handler.Options{
		GVKToRunner: watches,
	})
	if err != nil {
		logrus.Errorf("unable to create ansible handler - %v", err)
		done <- err
		return
	}
	sdk.Handle(h)
	sdk.Run(context.TODO())
	done <- nil
}
