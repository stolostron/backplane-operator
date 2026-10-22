# Component Onboarding Skills

This directory contains Claude Code skills for automating the onboarding of new components to the MCE (Multicluster Engine) operator.

## Available Skills

### 1. `/onboard-component` - Onboard from Local Template File

Automates component onboarding using a local YAML template file.

**Usage:**
```bash
/onboard-component <template-file-path>
```

**Example:**
```bash
/onboard-component .claude/examples/cluster-api-onboarding-template.yaml
```

**When to use:**
- You have a template file prepared locally
- Testing the onboarding process
- Working offline or without Jira access

### 2. `/onboard-component-from-jira` - Onboard from Jira Ticket

Automates component onboarding by reading the template from a Jira ticket description.

**Usage:**
```bash
/onboard-component-from-jira <JIRA-TICKET-ID>
```

**Example:**
```bash
/onboard-component-from-jira ACM-12345
```

**When to use:**
- Component onboarding request comes from a Jira ticket
- Want to automatically comment back to the ticket with results
- Working in a team environment with Jira tracking

**Requirements:**
- Jira ticket description must contain a YAML code block with the component template
- Jira credentials must be configured (via environment variables or settings)

## What These Skills Do

Both skills automate the complete component onboarding process by making code changes across multiple files:

### Code Changes Made

1. **`api/v1/multiclusterengine_methods.go`**
   - Add component name constants
   - Add CRD directory constants
   - Update component lists (AllComponents, MCEComponents, PreviewComponents)
   - Update preview-to-stable mappings

2. **`pkg/toggle/toggle.go`**
   - Add chart directory constants

3. **`controllers/toggle_components.go`**
   - Create `ensure<Component>()` function
   - Create `ensureNo<Component>()` function

4. **`controllers/backplaneconfig_controller.go`**
   - Add component toggle logic to reconciler
   - Update `fetchChartOrCRDPath()` function

5. **Bundle Automation Config**
   - `hack/bundle-automation/charts-config.yaml` (for Helm charts)
   - `hack/bundle-automation/config.yaml` (for bundles)

## Template Format

See `.claude/examples/cluster-api-onboarding-template.yaml` for a complete example.

### Required Fields

```yaml
metadata:
  name: "component-name"           # Required: kebab-case
  displayName: "Component Name"    # Required
  description: "..."               # Required

component:
  deploymentName: "..."            # Required: deployment name for status
  isPreview: false                 # Required
  hasPlatformVariants: false       # Required

source:
  type: "helm-chart"               # Required: "helm-chart" or "bundle"
  repository:
    url: "..."                     # Required
    branch: "..."                  # Required
```

### Optional Fields

```yaml
component:
  previewVariant: "..."            # If component has preview variant
  platformVariants:                # If hasPlatformVariants is true
    ocp:
      chartDir: "..."
      crdDir: "..."
    k8s:
      chartDir: "..."
      crdDir: "..."

source:
  helmChart:                       # For type: "helm-chart"
    path: "..."
  bundle:                          # For type: "bundle"
    path: "..."
  imageMappings: {}
  inclusions: []
  escapeTemplateVariables: []
  skipRBACOverrides: true
  updateChartVersion: true
  autoInstallForAllClusters: false

integration:
  supportsExternalManagement: true
  exclusions: []
  ignoreWebhookDefinitions: false
  preserveFiles: []
  webhookPaths: []
```

## Examples

See `.claude/examples/` directory for:

- **`cluster-api-onboarding-template.yaml`** - Complete example template file
- **`jira-ticket-template.md`** - Example Jira ticket descriptions

## Naming Conventions

### Component Names (in template)
- Use **kebab-case**: `cluster-api`, `managed-serviceaccount`

### Generated Go Constants
- Converted to **PascalCase** with special handling:
  - `api` → `API`
  - `aws` → `AWS`
  - `k8s` → `K8S`
  - `ocp` → `OCP`
  - `mce` → `MCE`

**Examples:**
- `cluster-api` → `ClusterAPI`
- `cluster-api-provider-aws` → `ClusterAPIProviderAWS`
- `managed-serviceaccount` → `ManagedServiceAccount`

## After Onboarding

After the skill completes, you must:

1. **Run code generation:**
   ```bash
   make generate
   ```

2. **Add chart/CRD files** to the appropriate directories:
   - Charts: `pkg/templates/charts/toggle/<component-name>/`
   - CRDs: `pkg/templates/crds/<component-name>/`

3. **Run tests:**
   ```bash
   make test
   ```

4. **Verify changes:**
   - Review all modified files
   - Ensure naming follows project conventions
   - Check that both OCP and K8s variants are handled (if applicable)

5. **Commit and create PR**

## Jira Integration

For the `/onboard-component-from-jira` skill to work, configure Jira credentials:

### Environment Variables
```bash
export JIRA_URL="https://issues.redhat.com"
export JIRA_TOKEN="your-personal-access-token"
```

### Or in Settings
Add to `.claude/settings.local.json`:
```json
{
  "env": {
    "JIRA_URL": "https://issues.redhat.com",
    "JIRA_TOKEN": "your-token"
  }
}
```

## Troubleshooting

### Skill not found
- Ensure the skill file exists in `.claude/skills/`
- File must have `.md` extension
- Check the skill frontmatter includes `skill: <name>`

### Template validation failed
- Check YAML syntax is valid
- Ensure all required fields are present
- Check component name is unique (doesn't already exist)

### Jira ticket not accessible
- Verify Jira credentials are configured
- Check ticket ID format (e.g., `ACM-12345`)
- Ensure you have permission to view the ticket

### Code changes incomplete
- Review the skill output for any errors
- Check if files are read-only or locked
- Verify repository is in clean state

## Contributing

When modifying these skills:

1. Update the skill documentation
2. Update examples if template format changes
3. Test with both simple and complex components
4. Update this README

## See Also

- [Component Onboarding Guide](../../docs/component-onboarding.md) *(if exists)*
- [MCE Development Guide](../../CONTRIBUTING.md)
- [Jira Integration](../../docs/jira.md) *(if exists)*
