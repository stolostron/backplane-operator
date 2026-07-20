---
skill: onboard-component-from-jira
description: Automates onboarding of a new component to the MCE operator from a Jira ticket
---

# Onboard Component from Jira

Automates the onboarding of a new component to the Multicluster Engine (MCE) operator by reading a component template from a Jira ticket description and making all necessary code changes.

## Usage

```
/onboard-component-from-jira <JIRA-TICKET-ID>
```

Example:
```
/onboard-component-from-jira ACM-12345
```

## Prerequisites

The Jira ticket description must contain a YAML template in a code block (fenced with triple backticks). The template should follow the component onboarding template format.

Example Jira ticket description:
````
## Component Onboarding Request

Please onboard the new component with the following configuration:

```yaml
metadata:
  name: "new-component"
  displayName: "New Component"
  description: "Description of the new component"

component:
  isPreview: false
  deploymentName: "new-component-manager"
  hasPlatformVariants: false
  # ... rest of template
```

## Additional Notes
Any additional context about the component...
````

## Instructions

When this skill is invoked with a Jira ticket ID:

1. **Fetch the Jira ticket** using the ticket ID:
   - Use the Jira API or CLI to fetch the ticket
   - Extract the ticket description
   - Look for YAML code blocks (between triple backticks with optional `yaml` language identifier)
   - Parse the first YAML code block found as the component template

2. **Validate the template** has all required fields:
   - metadata.name
   - metadata.displayName
   - component.deploymentName
   - source.type
   - source.repository

3. **Add component constants** to `api/v1/multiclusterengine_methods.go`:
   - Add component name constant (e.g., `NewComponent = "new-component"`)
   - Add preview variant constant if specified (e.g., `NewComponentPreview = "new-component-preview"`)
   - Add CRD directory constants (e.g., `NewComponentCRDDir = "new-component"`)
   - If component has platform variants, add both OCP and K8s CRD directory constants

4. **Update component lists** in `api/v1/multiclusterengine_methods.go`:
   - Add component to `AllComponents` slice
   - Add to `MCEComponents` slice (if not a preview-only component)
   - Add to `PreviewComponents` slice (if it's a preview component)
   - Add to `PreviewToStable` map (if component has both preview and stable variants)

5. **Add chart directory constants** to `pkg/toggle/toggle.go`:
   - Add chart directory constant(s) following the pattern `<ComponentName>ChartDir`
   - If platform variants exist, add both OCP and K8s chart directory constants

6. **Create toggle functions** in `controllers/toggle_components.go`:
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

7. **Integrate into main controller** in `controllers/backplaneconfig_controller.go`:
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

8. **Update fetchChartOrCRDPath function** in `controllers/backplaneconfig_controller.go`:
   - If component has platform variants:
     - Add variable declaration at the top of the function
     - Add OCP variant assignment in the `if utils.DeployOnOCP()` block
     - Add K8s variant assignment in the `else` block
     - Add switch case returning the appropriate chart location variable
   - If component has no platform variants:
     - Add simple switch case returning the chart directory constant

9. **Add bundle automation config**:
   - If source type is "helm-chart", update `hack/bundle-automation/charts-config.yaml`:
     - Add new chart entry under the appropriate repository or create new repository entry
     - Include all image mappings, inclusions, escape variables, etc.
   - If source type is "bundle", update `hack/bundle-automation/config.yaml`:
     - Add new operator entry with bundle path and configuration

10. **Add comment to Jira ticket**:
    - Post a comment to the Jira ticket with:
      - Summary of files modified
      - Branch name where changes were made
      - Link to the PR (if applicable)
      - Next steps (e.g., "Run `make generate` to update generated code")

## Fetching Jira Ticket

Use the Jira REST API to fetch the ticket. The API endpoint is typically:
```
https://<jira-domain>/rest/api/2/issue/<ticket-id>
```

Authentication can be done via:
- Personal Access Token (preferred)
- Basic auth with username/API token

Look for environment variables or settings:
- `JIRA_URL` or `JIRA_DOMAIN`
- `JIRA_TOKEN` or `JIRA_API_TOKEN`
- `JIRA_USER` or `JIRA_USERNAME`

If credentials are not available, ask the user to provide them or use the `gh` CLI if available.

## Template Format

The YAML template in the Jira description must follow this structure:

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

## Error Handling

If any of these conditions are met, stop and report the error:
- Jira ticket cannot be fetched (invalid ID, authentication failure, network error)
- No YAML code block found in the ticket description
- YAML is malformed or cannot be parsed
- Required template fields are missing
- Component name already exists in the codebase

## After Completion

After making all changes:
1. Report summary of all files modified
2. Component name and key details
3. Post comment to Jira ticket with summary
4. Remind to run `make generate` to update generated code
5. Remind to add actual chart/CRD files to the appropriate directories
6. Create or update PR linked to the Jira ticket
