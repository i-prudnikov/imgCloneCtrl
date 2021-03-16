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
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	crv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// withKind is used to enrich reconcile.Request
// with `kind` of object to be queued
// This does the trick to use the single handler with
// different objects (Deployment and DemonSet)
func withKind(obj client.Object) []reconcile.Request {
	kind := "DaemonSet"
	if _, isDeployment := obj.(*appsv1.Deployment); isDeployment {
		kind = "Deployment"
	}

	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Name:      fmt.Sprintf("%s:%s", kind, obj.GetName()),
			Namespace: obj.GetNamespace(),
		},
	}}
}

// reconciler reconciles Deployment & DaemonSet
type reconciler struct {
	// client can be used to retrieve objects from the APIServer.
	client             client.Client
	ignoredNamespaces  map[string]struct{} //set of ignored namespaces
	backupRegistry     string              //backup registry
	authConfig         authn.AuthConfig    //config to authn against backup registry

}

// Implement reconcile.Reconciler so the controller can reconcile objects
var _ reconcile.Reconciler = &reconciler{}

// fetchObjectFromRequest returns client.Object.
// Content of request is examined against holding either Deployment or DaemonSet.
func (r *reconciler) fetchObjectFromRequest(ctx context.Context, request reconcile.Request) (client.Object, error) {
	var (
		err error
		dp  = &appsv1.Deployment{}
		ds  = &appsv1.DaemonSet{}
		//obj = make([]client.Object, 0, len(mgdObjectsTypes))
		obj                   client.Object
		parts                 []string
		kind, name            string
		genericNamespacedName types.NamespacedName
	)

	//request.NamespacedName.Name holds kind:Name
	parts = strings.SplitN(request.NamespacedName.Name, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("could not parse kind and name from request.NamespacedName: %+v", request.NamespacedName)
	}

	kind = parts[0]
	name = parts[1]
	genericNamespacedName = types.NamespacedName{
		Name:      name,
		Namespace: request.NamespacedName.Namespace,
	}

	switch kind {
	case "Deployment":
		err = r.client.Get(ctx, genericNamespacedName, dp)
		obj = dp
	case "DaemonSet":
		err = r.client.Get(ctx, genericNamespacedName, ds)
		obj = ds
	}

	if errors.IsNotFound(err) {
		return nil, fmt.Errorf("could not find %s, %s", kind, genericNamespacedName.String())
	}
	if err != nil { //some other general errors
		return nil, fmt.Errorf("could not fetch %s: %+v", kind, err)
	}

	return obj, nil
}

// getTargetImage renders target image (using backup registry) from source image
// Note that destination image is flattened to have only name and tag, as backup
// registry can lack of support of nested registries
func (r *reconciler) getTargetImage(srcImageFull string) string {
	//parsing srcImageFull - splitting to registry, path, name & tag
	srcImageFullParts := strings.SplitN(srcImageFull, "/", 2)
	//srcImageRegistry := ""    // image registry name
	srcImagePathNameTag := "" //full image path, name & tag.

	//This code is borrowed from: https://github.com/moby/moby/blob/master/registry/service.go#L150
	if len(srcImageFullParts) == 1 || (!strings.Contains(srcImageFullParts[0], ".") &&
		!strings.Contains(srcImageFullParts[0], ":") && srcImageFullParts[0] != "localhost") {
		// This is a Docker Hub repository (ex: samalba/hipache or ubuntu),
		// use the default Docker Hub registry (docker.io)
		//srcImageRegistry = "docker.io"
		srcImagePathNameTag = srcImageFull
	} else {
		//srcImageRegistry = srcImageFullParts[0]
		srcImagePathNameTag = srcImageFullParts[1]
	}

	srcImagePathNameTagParts := strings.SplitN(srcImagePathNameTag, ":", 2)
	if len(srcImageFullParts) == 1 { //adding "latest" tag, if tag is not specified

		srcImagePathNameTagParts = append(srcImagePathNameTagParts, "latest")
	}

	//flatten path & name from service/platform/nginx => service_platform_nginx and move it to tag.
	//original tag added in the end after `_`
	return fmt.Sprintf("%s:%s_%s", r.backupRegistry, strings.ReplaceAll(srcImagePathNameTagParts[0], "/", "_"), srcImagePathNameTagParts[1])

}

// updateSpecWithImage updates images in an object spec
// 2 types of objects supported - Deployment and DaemonSet
// The function returns a mapping (map[string]string) that can determine for every
// source image it's destination (from backup registry) counterpart.
// If the image is already updated to backup registry, it is not added to the map
func (r *reconciler) updateSpecWithImage(obj client.Object) (map[string]string, error) {
	var (
		imageSrcDst = map[string]string{} //mapping of src image -> dst image
		podSpec     v1.PodSpec
	)
	switch obj.GetObjectKind().GroupVersionKind().Kind {
	case "Deployment":
		dp, isDeployment := obj.(*appsv1.Deployment)
		if !isDeployment {
			return nil, fmt.Errorf("object is not of type *appsv1.Deployment")
		}
		podSpec = dp.Spec.Template.Spec

	case "DaemonSet":
		ds, isDaemonSet := obj.(*appsv1.DaemonSet)
		if !isDaemonSet {
			return nil, fmt.Errorf("object is not of type *appsv1.DaemonSet")
		}
		podSpec = ds.Spec.Template.Spec
	}

	for i, c := range podSpec.Containers {
		if strings.Contains(c.Image, r.backupRegistry) { //already updated
			continue
		}
		imageSrcDst[c.Image] = r.getTargetImage(c.Image)
		podSpec.Containers[i].Image = imageSrcDst[c.Image]
	}

	for i, c := range podSpec.InitContainers {
		if strings.Contains(c.Image, r.backupRegistry) { //already updated
			continue
		}
		imageSrcDst[c.Image] = r.getTargetImage(c.Image)
		podSpec.InitContainers[i].Image = imageSrcDst[c.Image]
	}

	return imageSrcDst, nil
}

// pushImagesToBackupRegistry pushes image to backup registry
func (r *reconciler) pushImagesToBackupRegistry(ctx context.Context, imageSrcDst map[string]string) error {
	var (
		dstAuthOpts    remote.Option
		err            error
		srcRef, dstRef name.Reference
		srcImg         crv1.Image
	)

	lg := log.FromContext(ctx)

	for srcName, dstName := range imageSrcDst {
		srcRef, err = name.ParseReference(srcName)
		if err != nil {
			return fmt.Errorf("could not parse source image %q", srcName)
		}

		dstRef, err = name.ParseReference(dstName)
		if err != nil {
			return fmt.Errorf("could not parse destiantion image %q", srcName)
		}

		srcImg, err = remote.Image(srcRef, remote.WithAuth(authn.Anonymous), remote.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("could not get image %q from registry ", srcName)
		}

		//Check if backup repository has the source image already
		if dstImg, err := remote.Image(dstRef, remote.WithAuth(authn.Anonymous), remote.WithContext(ctx)); err == nil {

			if dstHash, err := dstImg.Digest(); err == nil {
				//Checking if dstHash is equal to srcHash
				if srcHash, err := srcImg.Digest(); err == nil && srcHash == dstHash {
					lg.Info(fmt.Sprintf("source image %q is already in backup registry", srcName))
					continue
				}
			}

		}

		//Authentication for backup registry
		if r.authConfig.Username == "" || r.authConfig.Password == "" {
			dstAuthOpts = remote.WithAuth(authn.Anonymous)
		} else {
			dstAuthOpts = remote.WithAuth(authn.FromConfig(r.authConfig))
		}

		lg.Info(fmt.Sprintf("pushing image %q to registry", dstName))
		if err := remote.Write(dstRef, srcImg, dstAuthOpts, remote.WithContext(ctx)); err != nil {
			return fmt.Errorf("could not push image %q to registry ", dstName)
		}
	}

	return nil
}

// Reconcile - primary handler for the controller objects. It receives requests
// pointing out to object, and process object according to controller logic
func (r *reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	var (
		obj client.Object
		err error
	)

	//Filter out based on namespace
	if _, ignore := r.ignoredNamespaces[request.Namespace]; ignore {
		return reconcile.Result{}, nil
	}

	// set up a convenient lg object so we don't have to type request over and over again
	lg := log.FromContext(ctx)

	//TODO: remove
	/*
	if request.Namespace != "test" {
		return reconcile.Result{}, nil
	}
	if request.Name != "Deployment:server" && request.Name != "DaemonSet:server" {
		return reconcile.Result{}, nil
	}*/

	//This returns managed object based on kind
	obj, err = r.fetchObjectFromRequest(ctx, request)
	if err != nil {
		lg.Error(err,"could not fetch object")
		return reconcile.Result{}, nil
	}

	//Update images in the spec, to use images from backup registry
	imageSrcDst, err := r.updateSpecWithImage(obj)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("could not update image in %s: %+v", obj.GetObjectKind().GroupVersionKind().Kind, err)
	}

	if len(imageSrcDst) == 0 { //Nothing to process
		lg.Info("already reconciled")
		return reconcile.Result{}, nil
	}

	//Pushing images to backup registry
	//This operation is time consuming and has 3rd party dep. It must respects the context
	lg.Info("start processing images...")
	err = r.pushImagesToBackupRegistry(ctx, imageSrcDst)
	if err != nil {
		return reconcile.Result{RequeueAfter: time.Second * 3}, fmt.Errorf("could not push images to remote registry (requied in 3 sec): %v", err)
	}

	//Commit changes in object spec
	err = r.client.Update(ctx, obj)
	if err != nil {
		return reconcile.Result{RequeueAfter: time.Second}, fmt.Errorf("could not write %s, (requied in 1 sec): %+v", obj.GetObjectKind().GroupVersionKind().Kind, err)
	}

	return reconcile.Result{}, nil
}
