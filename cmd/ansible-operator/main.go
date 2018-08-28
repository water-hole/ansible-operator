package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"runtime"
	"strings"
	"time"

	sdkVersion "github.com/operator-framework/operator-sdk/version"
	handler "github.com/water-hole/ansible-operator/pkg/handler"
	proxy "github.com/water-hole/ansible-operator/pkg/proxy"
	"github.com/water-hole/ansible-operator/pkg/runner"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
	"sigs.k8s.io/controller-runtime/pkg/source"

	crthandler "sigs.k8s.io/controller-runtime/pkg/handler"

	"github.com/sirupsen/logrus"
)

func printVersion() {
	logrus.Infof("Go Version: %s", runtime.Version())
	logrus.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	logrus.Infof("operator-sdk Version: %v", sdkVersion.Version)
}

func main() {
	flag.Parse()
	logf.SetLogger(logf.ZapLogger(false))

	mrg, err := manager.New(config.GetConfigOrDie(), manager.Options{})
	if err != nil {
		log.Fatal(err)
	}

	printVersion()
	done := make(chan error)

	// start the proxy
	proxy.RunProxy(done, proxy.Options{
		Address: "localhost",
		Port:    8888,
	})

	// start the operator
	go runSDK(done, mrg)

	// wait for either to finish
	err = <-done
	if err == nil {
		logrus.Info("Exiting")
	} else {
		logrus.Fatal(err.Error())
	}
}

func registerGVK(gvk schema.GroupVersionKind, mrg manager.Manager) {
	mrg.GetScheme().AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
}

// Used to set the decoder to just deserialize instead of decode.
// This is only need as a work around for now and when the SDK is based on
// controller-runtime we expect this to be fixed.
// This issue https://github.com/operator-framework/operator-sdk/issues/382
// is tracking this process.
func decoder(gv schema.GroupVersion, codecs serializer.CodecFactory) k8sruntime.Decoder {
	return codecs.UniversalDeserializer()
}

func runSDK(done chan error, mrg manager.Manager) {
	// setting the utility decoder function.
	//k8sutil.SetDecoderFunc(decoder)
	//namespace, err := k8sutil.GetWatchNamespace()
	//if err != nil {
	//	logrus.Error("Failed to get watch namespace")
	//	done <- err
	//	return
	//}
	//resyncPeriod := 60
	namespace := "default"
	watches, err := runner.NewFromWatches("/opt/ansible/watches.yaml")
	if err != nil {
		logrus.Error("Failed to get watches")
		done <- err
		return
	}
	rand.Seed(time.Now().Unix())

	for gvk, runner := range watches {
		logrus.Infof("Watching %s/%v, %s, %s", gvk.Group, gvk.Version, gvk.Kind, namespace)
		//registerGVK(gvk)
		//sdk.Watch(fmt.Sprintf("%v/%v", gvk.Group, gvk.Version), gvk.Kind, namespace, resyncPeriod)
		h := &handler.ReconcileAnsibleOperator{
			Client: mrg.GetClient(),
			GVK:    gvk,
			Runner: runner,
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
	log.Fatal(mrg.Start(signals.SetupSignalHandler()))
	done <- nil
}
