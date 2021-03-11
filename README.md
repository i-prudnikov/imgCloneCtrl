# Image clone controller

### TL;DR

__The problem:__
We have a Kubernetes cluster on which we can run applications. These applications will often use publicly available container images, like official images of popular programs, e.g. Jenkins, PostgreSQL, and so on. Since the images reside in repositories over which we have no control, it is possible that the owner of the repo deletes the image while our pods are configured to use it.
In the case of a subsequent node rotation, the locally cached copies of the images would be deleted and Kubernetes would be unable to re-download them in order to re-provision the applications.

__The goal:__
To be safe against the risk of public container images disappearing from the registry while we use them, breaking our deployments.

__The solution:__

[![asciicast](https://asciinema.org/a/poCTy7fPMsvHAT5lOATaMALtU.svg)](https://asciinema.org/a/poCTy7fPMsvHAT5lOATaMALtU)
---
#### IMPORTANT! Image transformation & backup registry
Before start using this controller, keep in mind the following approach taken for image transformation:
1. Since backup registry can be of choice, we assume that nested registries are not supported, i.e. 
backup image is flattened to have only name and tag.
2. By default, when image is pushed to backup repository, corresponding registry will be added automatically, however, that registry will be private by default. So you need to prepare appropriate image pull secret upfront. Otherwise you crash all your deployments and daemonsets. This is not optimal.
Thus, the controller built in a way that the target registry in the backup repository must be created upfront, with its visibility set to "public". And the images names itransformed to refer to it.
---

Usage instructions:
1. How to build:
- clone repo the repo
- in the project root run the following to build docker container:
```bash
git checkout v0.0.1
docker build -f ./deploy/Dockerfile ./ -t some.registry/image-clone-controller:0.0.1
docker push some.registry/image-clone-controller:0.0.1
```
__Note:__ Container is using `tini` supervisor.

2. How to deploy to k8s

- All manifests are in single file: `./deploy/deploy.yaml`
- Before deployment:
  * you need to update the deployment to use the image of controller from appropriate registry (see p. 1)
  * you need to set the name and credentials of your backup registry
    `Deployment->spec->template->spec->containers[]->args`    
```yaml
     containers:
        - name: controller
          args: [
              "imgCloneCtrl",
              "--ignoreNamespace=<NAMESPACE1>", #UPDATE THIS
              "--ignoreNamespace=<NAMESPACE2>", #UPDATE THIS
              "--backupRegistry=backup.repository/namespace/registry", #UPDATE THIS 
              "--backupRegistryUser=<YOUR_REGISTRY_USER>", #UPDATE THIS
              "--backupRegistryPassword=<YOUR_REGISTRY_PASSWORD>", #UPDATE THIS
              "--leaderElectionID=image-clone-controller-leader",
              "--leaderElectionNamespace=test-ki"]
          image: "some.registry/image-clone-controller:0.0.1" #UPDATE THIS
```
  For reference regarding controller command line flags reger to p.3

To deploy, you need to run the following command:
```bash
kubectl apply -f ./deploy/deploy.yaml
```

3. Controller cli

Cli interface is self-explanatory:
```bash
imgCloneCtrl -h 
Usage of ./build/imgCloneCtrl:
  -backupRegistry string
        Backup registry to use (i.e. quay.io/namespace/registry) NOTE! Here should be passed a repository, namespace and a registry.
        The registry is recommended to have puplic visibility.
  -backupRegistryPassword string
        Backup registry password
  -backupRegistryUser string
        Backup registry user
  -ignoreNamespace value
        Name of namespace to ignore. Multiple values supported. ('kube-system' is always ignored!)
  -kubeconfig string
        Paths to a kubeconfig. Only required if out-of-cluster.
  -leaderElectionID string
        Leader election ID (configmap with this name will be created)
  -leaderElectionNamespace string
        Election namespace - in which leader election ID config map will be created
  -version
        Print version
```
