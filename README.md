# kube-freeze-operator

Kubernetes operator that enforces **change freeze** and **maintenance windows**.

In v1.0 this repository targets:
- CRDs: `MaintenanceWindow`, `ChangeFreeze`, `FreezeException` (`freeze-operator.io/v1alpha1`)
- Enforcement: Validating Admission Webhook for Deployments/StatefulSets/DaemonSets/CronJobs
- Controller behavior: suspend/resume CronJobs while a policy is active (optional)

See the planning docs: [todo.md](todo.md) and [v2.0-v3.0-todo.md](v2.0-v3.0-todo.md).

## Status

Scaffolded via Kubebuilder. Core reconciliation/enforcement logic is work-in-progress.

## Getting Started

### Prerequisites
- Go 1.25.7 (recommended)
- Docker (for building/pushing the manager image)
- `kubectl`
- Access to a Kubernetes cluster
- **cert-manager** (required for webhook TLS; installation steps below)

### Install cert-manager (required)

This operator uses an Admission Webhook, which must serve HTTPS. We use cert-manager to:
- issue the webhook serving certificate (`kubernetes.io/tls` Secret)
- inject the CA bundle into `ValidatingWebhookConfiguration`

Install cert-manager (pick a version and keep it consistent across environments):

```sh
kubectl create namespace cert-manager --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.19.1/cert-manager.yaml
kubectl -n cert-manager rollout status deployment/cert-manager --timeout=5m
kubectl -n cert-manager rollout status deployment/cert-manager-webhook --timeout=5m
kubectl -n cert-manager rollout status deployment/cert-manager-cainjector --timeout=5m
```

Sanity check:

```sh
kubectl -n cert-manager get pods
```

### Deploy to a cluster

The default image is controlled by the `IMG` variable in the Makefile.

**Build and push your image to the registry specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/kube-freeze-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the controller-manager and webhooks to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/kube-freeze-operator:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Verify the deployment**

```sh
kubectl -n kube-freeze-operator-system get deploy,svc,pods
kubectl -n kube-freeze-operator-system get issuer,certificate,secret
kubectl get validatingwebhookconfigurations | grep kube-freeze-operator
```

The webhook Service should have endpoints, and the serving certificate Secret should exist:

```sh
kubectl -n kube-freeze-operator-system get endpoints kube-freeze-operator-webhook-service -o wide
```

**Create instances (CRs)**

You can apply the samples (examples) from `config/samples/`:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

## Validation scripts

There are end-to-end scripts under [hack](hack/) that redeploy the operator (optional) and validate admission behavior.

- MaintenanceWindow + workloads: [hack/validate_maintenancewindow.sh](hack/validate_maintenancewindow.sh)
- ChangeFreeze lifecycle: [hack/validate_changefreeze.sh](hack/validate_changefreeze.sh)
- FreezeException lifecycle: [hack/validate_freezeexception.sh](hack/validate_freezeexception.sh)

Common environment variables:

- `IMG` (default: `jamalshahverdiev/kube-freeze-operator:v1.0.4`) — operator image to deploy
- `REDEPLOY` (default: `true`) — whether to run `make deploy` before validations
- `PROD_NS` / `DEV_NS` — namespaces used by the script (names differ per script)

Examples:

```sh
# Run MaintenanceWindow end-to-end validation
bash hack/validate_maintenancewindow.sh

# Run without redeploy
REDEPLOY=false bash hack/validate_maintenancewindow.sh

# Validate a specific image tag
IMG=jamalshahverdiev/kube-freeze-operator:v1.0.4 bash hack/validate_changefreeze.sh
```

### Uninstall

**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Undeploy the controller/webhooks from the cluster:**

```sh
make undeploy
```

**Delete the APIs (CRDs) from the cluster:**

```sh
make uninstall
```

## Project Distribution

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/kube-freeze-operator:tag
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/kube-freeze-operator/<tag or branch>/dist/install.yaml
```

### By providing a Helm Chart

1. Build the chart using the optional helm plugin

```sh
kubebuilder edit --plugins=helm/v2-alpha
```

2. See that a chart was generated under 'dist/chart', and users
can obtain this solution from there.

**NOTE:** If you change the project, you need to update the Helm Chart
using the same command above to sync the latest changes. Furthermore,
if you create webhooks, you need to use the above command with
the '--force' flag and manually ensure that any custom configuration
previously added to 'dist/chart/values.yaml' or 'dist/chart/manager/manager.yaml'
is manually re-applied afterwards.

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

