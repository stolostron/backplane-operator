# Architecture: backplane-operator

## Overview

`backplane-operator` is the **MultiCluster Engine (MCE)** operator — the
engine layer beneath Red Hat Advanced Cluster Management (ACM). It is a
Kubernetes/OpenShift operator (module `github.com/stolostron/backplane-operator`,
API group `multicluster.openshift.io`) that manages installation, upgrade,
configuration, and lifecycle of the foundational components required to
manage multiple Kubernetes clusters from a central hub: cluster lifecycle
(Hive, HyperShift, Cluster API providers), registration/work/placement
(OCM), assisted installation, console, and more.

The primary custom resource is `MultiClusterEngine` (cluster-scoped,
shortname `mce`). An MCE instance can optionally be "managed by" ACM — the
`multiclusterhub-operator` sits above this operator and deploys/adopts an
MCE as a prerequisite for the full ACM hub stack.

## Repository Structure

| Path | Purpose |
|------|---------|
| `main.go` | Operator entrypoint: scheme registration, manager setup, TLS config, OLM version detection, CRD pre-application, webhook setup. |
| `api/v1/` | `MultiClusterEngine` and `InternalEngineComponent` API types, webhooks, generated deepcopy. |
| `controllers/` | Reconciliation logic: `backplaneconfig_controller.go` (main reconciler), `toggle_components.go` (per-component ensure/ensureNo functions), `networkpolicy.go`, `uninstall.go`, `crd_compatibility.go`, `mcewebhook/`. |
| `pkg/` | `rendering/` (Helm chart + CRD rendering), `status/` (status tracking), `toggle/`, `foundation/`, `hive/`, `manifest/`, `overrides/`, `utils/`, `version/`, `templates/` (embedded Helm charts + CRDs). |
| `config/` | Kustomize manifests: `crd/`, `default/`, `manager/`, `rbac/`, `webhook/`, `samples/`. |
| `bundle/` | OLM bundle (CSV, CRD, configmap, webhook service). |
| `hack/` | `bundle-automation/` (Python chart/bundle generation), `catalog/`, `scripts/`. |
| `build/` | `Dockerfile.prow`, `Dockerfile.rhtap` (Konflux), `Dockerfile.test.prow`. |
| `.tekton/`, `.github/` | Konflux pipelines and GitHub Actions. |

## Core Components

### The `MultiClusterEngine` API (`api/v1/multiclusterengine_types.go`)

- **`Spec`**: `AvailabilityConfig` (Basic/High), `NodeSelector`, `ImagePullSecret`, `Overrides` (per-component config, image pull policy), `Tolerations`, `TargetNamespace` (default `multicluster-engine`), `LocalClusterName`, `NetworkPolicies`.
- **Component config model**: `ComponentConfig` → `ConfigOverride.Deployments` → `DeploymentConfig.Containers` → `ContainerConfig.Env` — enables per-container environment variable overrides.
- **`Status`**: `Phase` (Progressing/Paused/Available/Uninstalling/Error/Updating), `Components []ComponentCondition`, `Conditions`, `CurrentVersion`, `DesiredVersion`.
- **`InternalEngineComponent`**: a lightweight namespaced marker CR created per component to signal downstream component operators to reconcile.

### Managed components (`api/v1/multiclusterengine_methods.go`)

Includes assisted-service, cluster-api (+ AWS/Azure/Metal3/OpenShift-Assisted providers), cluster-lifecycle, cluster-manager, cluster-permission, cluster-proxy-addon, console-mce, discovery, hive, hypershift, image-based-install-operator, local-cluster, managedserviceaccount, server-foundation, and maestro. A `PreviewToStable` map auto-upgrades preview components to stable during reconciliation.

### Controllers

- **`MultiClusterEngineReconciler`** — the main controller (`controllers/backplaneconfig_controller.go`), with ~98 methods covering defaulting, CRD rendering, per-component toggling, status aggregation, and cleanup.
- **`mcewebhook.Reconciler`** — manages the webhook and serving certs on non-OpenShift clusters.
- **Validating/defaulting webhooks** on `MultiClusterEngine` — validate namespace, availability, deploy mode, component exclusivity, and block deletion when guarded resources exist.

### Status handling (`pkg/status/`)

A `StatusTracker` aggregates per-component `StatusReporter`s plus conditions to compute overall phase and availability; `CurrentVersion` only advances once the phase is `Available` (upgrade gating).

## Data / Control Flow

### Startup (`main.go`)

Registers schemes for OCP, Hive, OLM, OCM, and Prometheus APIs; detects
OpenShift and fetches the cluster's TLS security profile to configure the
webhook server; detects OLM v0 vs v1; pre-applies all component CRDs from
embedded templates (skipping CRDs marked to be ignored); wires the
reconciler and, on non-OpenShift, the serving-cert controllers.

### Reconcile loop (`controllers/backplaneconfig_controller.go`)

1. Fetch the `MultiClusterEngine` CR; reset failure conditions.
2. Handle deletion (finalizer cleanup) or apply defaults (availability, target namespace, default components).
3. Validate namespace, image pull secret, and minimum OCP version (skippable via annotation/env).
4. Resolve image/template overrides from environment variables, annotations, and developer ConfigMaps.
5. Honor a pause annotation.
6. Render and apply CRDs (skipping externally-managed or disabled-component CRD dirs).
7. Deploy always-on subcomponents, then loop over toggleable components calling `ensure<Component>` or `ensureNo<Component>` depending on enablement.
8. Manage NetworkPolicies (create-once pattern), trust bundle configmap, and metrics.
9. Update status and requeue.

### Component deployment mechanics

Charts are rendered via `pkg/rendering/renderer.go` (Helm v3 engine) with
global and hub-specific values (image/template overrides, node selectors,
tolerations, proxy config). Resources are applied via **server-side apply**
with field manager `backplane-operator`, gated by resource-ownership/adoption
policy and release-version alignment annotations.

### Override mechanisms

- **Images**: `OPERAND_IMAGE_*`/`RELATED_IMAGE_*` env vars, `image-repository` annotation, `image-overrides-configmap` annotation.
- **Templates**: `TEMPLATE_OVERRIDE_*` env vars, `template-override-configmap` annotation.
- **Per-component**: `spec.overrides.components[].configOverrides`.

## Build, Test & Release

- **Makefile**: `manifests`/`generate` (controller-gen), `test` (envtest + Ginkgo, coverage to `cover.out`), `build`/`run`, `docker-build`/`podman-build`, `install`/`deploy` (kustomize), `bundle`/`bundle-build`/`catalog-build` (OLM), `functional-tests` (Ginkgo, tag `functional`).
- **Dockerfiles**: `Dockerfile` (community, `golang` builder → `ubi9-minimal`); `build/Dockerfile.rhtap` (Konflux/RHTAP production build).
- **operator-sdk / kubebuilder**: kubebuilder v4 layout; CSV in `bundle/manifests/`.
- **CI**: `.tekton/` Konflux PipelineRuns per release branch (`backplane-X.Y`), multi-arch and hermetic; `.github/workflows/` includes RBAC-generation verification, chart/bundle regeneration, image-key validation, and OWNERS resync.
- **`hack/bundle-automation/`**: Python tooling that pulls component charts/bundles from upstream repos into `pkg/templates/charts` and regenerates the CSV — this is the integration point with `installer-dev-tools`' bundle-generation scripts.

## Dependencies & Integrations

- **controller-runtime**, **k8s.io** client libraries, **helm.sh/helm/v3** (chart rendering engine).
- **operator-framework/api** and **operator-lib** (OLM v0 `OperatorCondition`/Upgradeable).
- **open-cluster-management.io/api** and **sdk-go** (ClusterManager, addon API, serving certs).
- **openshift/api**, **openshift/hive/apis**.
- **Testing**: `onsi/ginkgo/v2` + `gomega`, `controller-runtime/pkg/envtest`.
- Component operands are external images/charts synced into `pkg/templates/charts` by the bundle-automation tooling in `installer-dev-tools`.

## Conventions & Patterns

- **Toggle pattern**: every component has paired `ensure<Component>`/`ensureNo<Component>` methods, driven by `spec.overrides.components[].enabled`.
- **Preview → stable lifecycle**: components auto-promote and get pruned on upgrade.
- **Server-side apply** with a dedicated field manager, ownership/adoption gating, and a release-version annotation for upgrade tracking.
- **Dual-platform support**: OpenShift vs. vanilla Kubernetes, with separate CAPI chart/CRD variants and conditional OLM v0/v1 branching.
- **Annotations namespace**: `installer.multicluster.openshift.io/*` for pause, overrides, and adoption policy; `multiclusterengine.openshift.io/ignore` to freeze a CRD.
- **Testing**: Ginkgo/Gomega BDD style with envtest for controller/webhook suites; separate functional test suite.
- **Versioning**: `OPERATOR_VERSION` env var is mandatory at startup; minimum supported OCP version enforced (skippable for development).
