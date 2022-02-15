[comment]: # ( Copyright Contributors to the Open Cluster Management project )

# Backplane Operator

Operator for managing installation of Backplane components

## Prerequisites

- Go v1.17+
- kubectl 1.19+
- Operator-sdk v1.10.0+
- Docker
- Connection to an existing Kubernetes cluster

## Installation

Before deploying, the CRDs need to be installed onto the cluster.

```shell
make install
```

### Outside the Cluster

The operator can be run locally against the configured Kubernetes cluster in ~/.kube/config with the following command:

```shell
make run
```

### Inside the Cluster

The operator can also run inside the cluster as a Deployment. To do that first build the container image and push to an accessible image registry:

1. Build the image:
    ```shell
    make docker-build IMG=<registry>/<imagename>:<tag>
    ```
2. Push the image:
    ```shell
    make docker-push IMG=<registry>/<imagename>:<tag>
    ```
3. Deploy the Operator:
    ```shell
    make deploy IMG=<registry>/<imagename>:<tag>
    ```
