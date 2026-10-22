# backplane-operator — Agent Instructions

This repository contains the backplane-operator (Multicluster Engine operator) for Red Hat Advanced Cluster Management (RHACM) and Multicluster Engine (MCE).

## What this operator does

The backplane-operator manages the foundational components of the MCE platform, including:

- Cluster lifecycle management (Hive, assisted-service, cluster-curator)
- Cluster registration and addon management (OCM hub components)
- Application lifecycle components (channels, subscriptions)
- Governance policy framework
- Managed cluster import and discovery

## Repository layout

- `api/` - CRD definitions for MultiClusterEngine
- `controllers/` - Operator reconciliation logic
- `pkg/` - Shared packages (rendering, status, helpers)
- `templates/` - Helm charts and manifests for MCE components
- `test/` - Integration and functional tests
- `config/` - Operator deployment manifests (CRDs, RBAC, deployment)

## Development workflow

### Building locally

```bash
make build        # Build operator binary
make docker-build # Build operator image
```

### Running locally

```bash
# Run operator outside cluster (for development)
make run

# Deploy operator to cluster
make deploy
```

### Testing

```bash
make test              # Run unit tests
make functional-test   # Run functional tests
```

## Dependencies

- **OpenShift 4.x** - Target platform
- **Operator SDK** - Operator framework
- **Helm** - Template rendering
- **Kustomize** - Manifest generation
- **Hive** - Cluster provisioning
- **OCM** - Open Cluster Management APIs

## Documentation

- [MCE API Reference](https://access.redhat.com/documentation/en-us/red_hat_advanced_cluster_management_for_kubernetes/)
- [MCE Component Registry](https://github.com/stolostron/acm-config/blob/main/product/component-registry.yaml)
- [Operator Development Guide](docs/)

## Common tasks

### Deploy a development build

```bash
export QUAY_USER=<your-quay-username>
make docker-build docker-push deploy
```

### Test MCE resource changes

```bash
# Edit sample MCE
vi config/samples/multicluster_engine_v1_multiclusterengine.yaml

# Apply to cluster
oc apply -f config/samples/multicluster_engine_v1_multiclusterengine.yaml
```

### Debug operator logs

```bash
oc logs -n multicluster-engine deployment/backplane-operator -f
```
