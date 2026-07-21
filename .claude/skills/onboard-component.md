---
skill: onboard-component
description: Automates onboarding of a new component to the MCE operator
---

# Onboard Component

Automates the onboarding of a new component to the Multicluster Engine (MCE) operator by reading a component template file and making all necessary code changes.

## Usage

```
/onboard-component <template-file>
```

## Instructions

When this skill is invoked with a template file path:

1. **Read and parse the template file** - The template is a YAML file describing the component to onboard

2. **Add component constants** to `api/v1/multiclusterengine_methods.go`:
   - Add component name constant (e.g., `ClusterAPI = "cluster-api"`)
   - Add preview variant constant if specified (e.g., `ClusterAPIPreview = "cluster-api-preview"`)
   - Add CRD directory constants (e.g., `ClusterAPICRDDir = "cluster-api"`)
   - If component has platform variants, add both OCP and K8s CRD directory constants

3. **Update component lists** in `api/v1/multiclusterengine_methods.go`:
   - Add component to `AllComponents` slice
   - Add to `MCEComponents` slice (if not a preview-only component)
   - Add to `PreviewComponents` slice (if it's a preview component)
   - Add to `PreviewToStable` map (if component has both preview and stable variants)

4. **Add chart directory constants** to `pkg/toggle/toggle.go`:
   - Add chart directory constant(s) following the pattern `<ComponentName>ChartDir`
   - If platform variants exist, add both OCP and K8s chart directory constants

5. **Create toggle functions** in `controllers/toggle_components.go`:
   - Create `ensure<ComponentName>` function that:
     - Creates NamespacedName using the deploymentName from template
     - Updates status manager
     - Calls `ensureInternalEngineComponent`
     - Renders charts using `fetchChartOrCRDPath`
     - Applies component deployment overrides
     - Applies all templates
   - Create `ensureNo<ComponentName>` function that:
     - Deletes InternalHubComponent
     - Renders charts
     - Deletes all templates
     - Updates status manager

6. **Integrate into main controller** in `controllers/backplaneconfig_controller.go`:
   - Find the component toggle section in the reconcile loop
   - Add the component toggle logic following the pattern:
     ```go
     if !r.isComponentExternallyManaged(backplaneConfig, backplanev1.<ComponentName>) {
         if backplaneConfig.Enabled(backplanev1.<ComponentName>) {
             result, err = r.ensure<ComponentName>(ctx, backplaneConfig)
             if result != (ctrl.Result{}) {
                 requeue = true
             }
             if err != nil {
                 errs[backplanev1.<ComponentName>] = err
             }
         } else {
             result, err = r.ensureNo<ComponentName>(ctx, backplaneConfig)
             if result != (ctrl.Result{}) {
                 requeue = true
             }
             if err != nil {
                 errs[backplanev1.<ComponentName>] = err
             }
         }
     } else {
         log.Info(messages.SkippingExternallyManaged, "component", backplanev1.<ComponentName>)
     }
     ```

7. **Update fetchChartOrCRDPath function** in `controllers/backplaneconfig_controller.go`:
   - If component has platform variants:
     - Add variable declaration at the top of the function
     - Add OCP variant assignment in the `if utils.DeployOnOCP()` block
     - Add K8s variant assignment in the `else` block
     - Add switch case returning the appropriate chart location variable
   - If component has no platform variants:
     - Add simple switch case returning the chart directory constant

8. **Add bundle automation config**:
   - If source type is "helm-chart", update `hack/bundle-automation/charts-config.yaml`:
     - Add new chart entry under the appropriate repository or create new repository entry
     - Include all image mappings, inclusions, escape variables, etc.
   - If source type is "bundle", update `hack/bundle-automation/config.yaml`:
     - Add new operator entry with bundle path and configuration

## Template Format

The template file must be a YAML file with this structure:

```yaml
metadata:
  name: "component-name"          # kebab-case component name
  displayName: "Component Name"   # Human-readable name
  description: "Component description"

component:
  isPreview: false                # Whether this is a preview component
  previewVariant: "component-name-preview"  # Optional: preview variant name
  deploymentName: "deployment-name"  # Deployment name for status tracking
  hasPlatformVariants: false      # Whether component has OCP/K8s variants
  
  platformVariants:               # Only if hasPlatformVariants is true
    ocp:
      chartDir: "pkg/templates/charts/toggle/component"
      crdDir: "component"
    k8s:
      chartDir: "pkg/templates/charts/toggle/component-k8s"
      crdDir: "component-k8s"

source:
  type: "helm-chart"              # "helm-chart" or "bundle"
  
  repository:
    url: "https://github.com/org/repo.git"
    branch: "backplane-5.0"
  
  helmChart:                      # For helm-chart type
    path: "charts/component"
  
  # OR for bundle type:
  # bundle:
  #   path: "deploy/manifests/"
  
  imageMappings:                  # Image name mappings
    image-name: "image_key"
  
  inclusions:                     # Template inclusions
    - "pullSecretOverride"
  
  escapeTemplateVariables:        # Variables to escape
    - "CLUSTER_NAME"
  
  skipRBACOverrides: true
  updateChartVersion: true
  autoInstallForAllClusters: false

integration:
  supportsExternalManagement: true
  exclusions: []                  # For bundle type
  ignoreWebhookDefinitions: false # For bundle type
  preserveFiles: []               # For bundle type
  webhookPaths: []                # For bundle type
```

## Naming Conventions

- **Component names**: Use kebab-case (e.g., `cluster-api`, `cluster-api-provider-aws`)
- **Go constants**: Convert to PascalCase, with special handling for:
  - `api` → `API`
  - `aws` → `AWS`
  - `k8s` → `K8S`
  - `ocp` → `OCP`
  - `mce` → `MCE`
  - Example: `cluster-api` → `ClusterAPI`, `cluster-api-provider-aws` → `ClusterAPIProviderAWS`

## After Completion

After making all changes, report:
1. Summary of all files modified
2. Component name and key details
3. Reminder to run `make generate` to update generated code
4. Reminder to add actual chart/CRD files to the appropriate directories
