

# Backplane Operator

Operator for managing installation of Backplane components

## Prerequisites

- Go v1.21+
- kubectl 1.19+
- Operator-sdk v1.17.0+
- Docker
- Connection to an existing Kubernetes cluster

## Installation

Before deploying, the CRDs need to be installed onto the cluster.

```shell {"id":"01HZ82TS792J05WVSFYZBE9RRX"}
make install
```

### Outside the Cluster

The operator can be run locally against the configured Kubernetes cluster in ~/.kube/config with the following command:

```shell {"id":"01HZ82TS792J05WVSFZ104WMK7"}
make run
```

### Inside the Cluster

The operator can also run inside the cluster as a Deployment. To do that first build the container image and push to an accessible image registry:

1. Build the image:

```shell {"id":"01HZ82TS792J05WVSFZ2QW3CBD"}
make docker-build IMG=<registry>/<imagename>:<tag>
```

2. Push the image:

```shell {"id":"01HZ82TS792J05WVSFZ3YHEZR0"}
make docker-push IMG=<registry>/<imagename>:<tag>
```

3. Deploy the Operator:

```shell {"id":"01HZ82TS792J05WVSFZ57BB0TF"}
make deploy IMG=<registry>/<imagename>:<tag>
```

Rebuild Date: Fri May 31 15:43:31 EDT 2024
