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
              "--ignoreNamespace=test2",
              "--backupRegistry=backup.registry/store", #UPDATE THIS
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
        Backup registry to use (i.e. quay.io/my_favorite_registry)
  -backupRegistryPassword string
        Backup registry password
  -backupRegistryUser string
        Backup registry user
  -ignoreNamespace value
        Name of namespace to ignore. Multiple values supported. (default kube-system)
  -kubeconfig string
        Paths to a kubeconfig. Only required if out-of-cluster.
  -leaderElectionID string
        Leader election ID (configmap with this name will be created)
  -leaderElectionNamespace string
        Election namespace - in which leader election ID config map will be created
  -version
        Print version
```
