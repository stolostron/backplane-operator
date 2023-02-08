## Modify CRDs deployed by operator

The MCE operator deploys several CRDs as soon as the container starts up. This happens outside the lifecycle of any multiclusterengine resource, so it can't be disabled via configuration. The MCE requires some of these CRDs in order to run properly, but in some cases it may be desirable to use a different version of the CRD. 

The MCE operator will always overwrite the existing CRD on startup by applying the version it has saved. To prevent this add an annotation on the CRD so the operator will not reapply it if it is already present.

Run the following example to annotate an existing multiclusterengine and prevent overwrite

```bash
kubectl annotate crd <crd-name> multiclusterengine.openshift.io/ignore=""
```

To remove this annotation
```bash
kubectl annotate crd <crd-name> multiclusterengine.openshift.io/ignore- --overwrite
```
