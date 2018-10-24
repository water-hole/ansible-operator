package main

import (
	"flag"
	"log"
	"runtime"

	"github.com/operator-framework/operator-sdk/pkg/ansible/operator"
	proxy "github.com/operator-framework/operator-sdk/pkg/ansible/proxy"
	"github.com/operator-framework/operator-sdk/pkg/util/k8sutil"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

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

	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		log.Fatalf("failed to get watch namespace: %v", err)
	}

	mgr, err := manager.New(config.GetConfigOrDie(), manager.Options{
		Namespace: namespace,
	})
	if err != nil {
		log.Fatal(err)
	}

	printVersion()
	done := make(chan error)

	// start the proxy
	err = proxy.Run(done, proxy.Options{
		Address:    "localhost",
		Port:       8888,
		KubeConfig: mgr.GetConfig(),
	})
	if err != nil {
		logrus.Fatalf("error starting proxy: %v", err)
	}

	// start the operator
	go operator.Run(done, mgr, "/opt/ansible/watches.yaml")

	// wait for either to finish
	err = <-done
	if err == nil {
		logrus.Info("Exiting")
	} else {
		logrus.Fatal(err.Error())
	}
}
