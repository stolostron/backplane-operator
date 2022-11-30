## Available Overrides / Development Tools

### Override Image Values

See [Overriding Images](override-images.md ) for details about modifying images at runtime

### Disable MCE Operator

Once installed, the mce operator will monitor changes in the cluster that affect an instance of the mce and reconcile deviations to maintain desired state. To stop the operator from making these changes you can apply an annotation to the mce instance.
```bash
kubectl annotate mce <mce-name> pause=true
```

Remove or edit this annotation to resume operator reconciliation
```bash
kubectl annotate mce <mce-name> pause- --overwrite
```

### Skip OCP Version Requirement

The operator defines a minimum version of OCP it can run in to avoid unexpected behavior. If the OCP environment is below this threshold then the MCE instance will report failure early on. This requirement can be ignored in the following two ways

1. Set `DISABLE_OCP_MIN_VERSION` as an environment variable. The presence of this variable in the container the operator runs will skip the check.

2. Set `ignoreOCPVersion` annotation in the MCE instance.
```bash
kubectl annotate mce <mce-name> ignoreOCPVersion=true
```