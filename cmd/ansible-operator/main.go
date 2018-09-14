package main

import (
	"flag"
	"log"
	"math/rand"
	"runtime"
	"time"

	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/water-hole/ansible-operator/pkg/controller"
	proxy "github.com/water-hole/ansible-operator/pkg/proxy"
	"github.com/water-hole/ansible-operator/pkg/runner"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	"github.com/sirupsen/logrus"
	"os"
	"strconv"
)

func printVersion() {
	logrus.Infof("Go Version: %s", runtime.Version())
	logrus.Infof("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
	logrus.Infof("operator-sdk Version: %v", sdkVersion.Version)
}

func main() {
	flag.Parse()
	logf.SetLogger(logf.ZapLogger(false))

	syncPeriod := time.Duration(60 * time.Second)

	if r, ok := os.LookupEnv("RESYNC_PERIOD"); ok {
		if rp, err := strconv.Atoi(r); err == nil {
			syncPeriod = time.Duration(rp * int(time.Second))
		}
	}

	mgr, err := manager.New(config.GetConfigOrDie(), manager.Options{SyncPeriod: &syncPeriod})

	if err != nil {
		log.Fatal(err)
	}

	printVersion()
	done := make(chan error)

	// start the proxy
	proxy.RunProxy(done, proxy.Options{
		Address:    "localhost",
		Port:       8888,
		KubeConfig: mgr.GetConfig(),
	})

	// start the operator
	go runSDK(done, mgr)

	// wait for either to finish
	err = <-done
	if err == nil {
		logrus.Info("Exiting")
	} else {
		logrus.Fatal(err.Error())
	}
}

func runSDK(done chan error, mgr manager.Manager) {
	namespace := "default"
	watches, err := runner.NewFromWatches("/opt/ansible/watches.yaml")
	if err != nil {
		logrus.Error("Failed to get watches")
		done <- err
		return
	}
	rand.Seed(time.Now().Unix())

	for gvk, runner := range watches {
		controller.Add(mgr, controller.Options{
			GVK:       gvk,
			Namespace: namespace,
			Runner:    runner,
		})
	}
	log.Fatal(mgr.Start(signals.SetupSignalHandler()))
	done <- nil
}
