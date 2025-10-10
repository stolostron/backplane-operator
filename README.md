

# Backplane Operator

Operator for managing installation of Backplane components

## Prerequisites

- Go v1.24.0+
- kubectl 1.19+
- Operator-sdk v1.17.0+
- Docker or Podman
- Connection to an existing Kubernetes cluster

## Installation

Before deploying, the CRDs need to be installed onto the cluster.

```bash
make install
```

### Outside the Cluster

The operator can be run locally against the configured Kubernetes cluster in ~/.kube/config with the following command:

```bash
make run
```

### Inside the Cluster

The operator can also run inside the cluster as a Deployment. To do that first build the container image and push to an accessible image registry:

1. Build the image:

```bash
make docker-build IMG=<registry>/<imagename>:<tag>
# or
make podman-build IMG=<registry>/<imagename>:<tag>
```

2. Push the image:

```bash
make docker-push IMG=<registry>/<imagename>:<tag>
# or
make podman-push IMG=<registry>/<imagename>:<tag>
```

3. Deploy the Operator:

```bash
make deploy IMG=<registry>/<imagename>:<tag>
```
