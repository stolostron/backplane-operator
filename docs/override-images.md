## Replace image repository

You can replace the repository of image references with an annotation in the multiclusterengine. This could be useful if you mirrored images to a new repository.

Here is an example multiclusterengine with the annotation

```yaml
apiVersion: multicluster.openshift.io/v1
kind: MultiClusterEngine
metadata:
  name: multiclusterengine
  annotations:
    "imageRepository": "quay.io/stolostron"
```

Run the following example to annotate an existing multiclusterengine and overwrite images with `quay.io/stolostron`

```bash
kubectl annotate mce <mce-name> --overwrite imageRepository="quay.io/stolostron"
```

## Replace images with Configmap

Images replacements can be defined in a configmap and referenced in the multiclusterengine resource. The operator will then deploy resources using these images. 

This is done by creating a configmap from a new [manifest](https://github.com/stolostron/backplane-pipeline/tree/2.2-integration/snapshots). A developer may use this to override any 1 or all images. This configmap must be in the same namespace as the backplane operator.


If overriding individual images, the minimum required parameters required to build the image reference are - 

- `image-name`
- `image-remote`
- `image-key`
- `image-digest` or `image-tag`, both can optionally be provided, if so the `image-digest` will be preferred.


```bash
kubectl create configmap <my-config> --from-file=docs/examples/image-override.json # Override 1 image example
kubectl annotate mce <mce-name> --overwrite imageOverridesCM=<my-config> # Provide the configmap name in an annotation
```

To remove this annotation to revert back to the original manifest
```bash
kubectl annotate mce <mce-name> imageOverridesCM- --overwrite # Remove annotation
kubectl delete configmap <my-config> # Delete configmap
```

To create the configmap directly without a json manifest modify and run the following in the backplane operator namespace:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: v1
data:
  manifest.json: |-
    [
      {
        "image-name": "discovery-operator",
        "image-remote": "quay.io/stolostron",
        "image-digest": "sha256:9dc4d072dcd06eda3fda19a15f4b84677fbbbde2a476b4817272cde4724f02cc",
        "image-key": "discovery_operator"
      }
    ]
kind: ConfigMap
metadata:
  name: my-config
EOF
```
