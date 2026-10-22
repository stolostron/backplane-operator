# CLAUDE.md

@AGENTS.md

## Build commands

```bash
make build         # Build operator binary
make docker-build  # Build operator image
make bundle        # Generate operator bundle
```

## Test commands

```bash
make test              # Run unit tests
make functional-test   # Run functional tests
make lint              # Run linters
```

## Local development

### Run operator locally

```bash
# Install CRDs
make install

# Run operator outside cluster
make run
```

### Deploy to cluster

```bash
# Deploy operator
make deploy

# Create MCE instance
oc apply -f config/samples/multicluster_engine_v1_multiclusterengine.yaml
```
