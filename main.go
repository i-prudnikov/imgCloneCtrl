/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"github.com/google/go-containerregistry/pkg/authn"
	appsv1 "k8s.io/api/apps/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	version string
	commit  string
	branch  string
)

func init() {

	log.SetLogger(zap.New(func(o *zap.Options) {
		o.Development = true
	}))

}

func main() {
	entryLog := log.Log.WithName("entrypoint")

	//Parsing command line parameters (defined in config.go)
	flag.Parse()
	if argPrintVersion {
		fmt.Printf("Image clone controller: version: %s, branch: %s, commit: %s\n", version,branch,commit)
		return
	}

	entryLog.Info("namespaces to ignore: " + argIgnoreNamespaces.String())
	if argBackupRegistry == "" {
		entryLog.Error(nil, "backup registry is not specified!")
		flag.Usage()
		os.Exit(1)
	} else {
		//TODO(i-prudnikov): Check validity of argBackupRegistry
		entryLog.Info("using backup registry: " + argBackupRegistry)
	}

	//TODO (i-prudnikov): Check for validity
	if argLeaderElectionID == ""  {
		entryLog.Error(nil, "--leaderElectionID is not specified!")
		flag.Usage()
		os.Exit(1)
	}

	//TODO (i-prudnikov): check for validity
	if argLeaderElectionNamespace == "" {
		entryLog.Error(nil, "--leaderElectionNamespace is not specified!")
		flag.Usage()
		os.Exit(1)
	}

	// Setup a Manager
	entryLog.Info("setting up manager")
	//TODO (i-prudnikov): Switch off leader election if LeaderElectionID is not provided
	mgr, err := manager.New(config.GetConfigOrDie(), manager.Options{
		LeaderElection: true,
		LeaderElectionID: argLeaderElectionID,
		LeaderElectionNamespace: argLeaderElectionNamespace,
	})
	if err != nil {
		entryLog.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	// Setup a new controller to reconcile Deployments & DaemonSets
	entryLog.Info("setting up controller")
	ctrl, err := controller.New("ImgCloneCtrl", mgr, controller.Options{
		Reconciler: &reconciler{
			client:            mgr.GetClient(),
			ignoredNamespaces: argIgnoreNamespaces,
			backupRegistry:    argBackupRegistry,
			authConfig: authn.AuthConfig{
				Username: argBackupRegistryUser,
				Password: argBackupRegistryPassword,
			},
		},
	})

	if err != nil {
		entryLog.Error(err, "unable to set up image clone controller")
		os.Exit(1)
	}

	//NOTE!
	//The following Watch calls uses handler.EnqueueRequestsFromMapFunc, that
	//enriches reconcile.Request with `kind` information about the object
	//This allows to use the single handler for both Deployment and DaemonSet.
	//The standard approach would be to create different handlers per every object,
	//but this lead to duplication of code.

	// Watch Deployment and enqueue object key (enriched with object kind)
	if err := ctrl.Watch(&source.Kind{Type: &appsv1.Deployment{}}, handler.EnqueueRequestsFromMapFunc(withKind)); err != nil {
		entryLog.Error(err, "unable to watch Deployments")
		os.Exit(1)
	}

	// Watch DaemonSet and enqueue object key (enriched with object kind)
	if err := ctrl.Watch(&source.Kind{Type: &appsv1.DaemonSet{}}, handler.EnqueueRequestsFromMapFunc(withKind)); err != nil {
		entryLog.Error(err, "unable to watch DaemonSets")
		os.Exit(1)
	}

	entryLog.Info("starting manager")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		entryLog.Error(err, "unable to run manager")
		os.Exit(1)

	}
}
