// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"fmt"
	"strings"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	/*
		AnnotationMCEIgnore labels a resource as something the operator should ignore and not update.
	*/
	AnnotationMCEIgnore = "multiclusterengine.openshift.io/ignore"

	/*
		AnnotationIgnoreOCPVersion is an annotation used to indicate the operator should not check the OpenShift
		Container Platform (OCP) version before proceeding when set.
	*/
	AnnotationIgnoreOCPVersion           = "installer.multicluster.openshift.io/ignore-ocp-version"
	DeprecatedAnnotationIgnoreOCPVersion = "ignoreOCPVersion"

	/*
		AnnotationEdgeManagementEnabled is an annotation used in multiclusterhub to whether the component edge manager
		is enabled or not.
	*/
	AnnotationEdgeManagerEnabled = "installer.open-cluster-management.io/edge-manager-enabled"

	/*
		AnnotationImageOverridesCM is an annotation used in multiclusterengine to specify a custom ConfigMap containing
		image overrides.
	*/
	AnnotationImageOverridesCM           = "installer.multicluster.openshift.io/image-overrides-configmap"
	DeprecatedAnnotationImageOverridesCM = "imageOverridesCM"

	/*
		AnnotationImageRepo is an annotation used in multiclusterengine to specify a custom image repository to use.
	*/
	AnnotationImageRepo           = "installer.multicluster.openshift.io/image-repository"
	DeprecatedAnnotationImageRepo = "imageRepository"

	/*
		AnnotationEditable is an annotation used on specific resources deployed by the hub to mark them as able
		to be ended by customer without being overridden.
	*/
	AnnotationEditable = "installer.multicluster.openshift.io/is-editable"

	/*
		AnnotationKubeconfig is an annotation used to specify the secret name residing in target containing the
		kubeconfig to access the remote cluster.
	*/
	AnnotationKubeconfig           = "installer.multicluster.openshift.io/kubeconfig"
	DeprecatedAnnotationKubeconfig = "mce-kubeconfig"

	/*
		AnnotationMCEPause is an annotation used in multiclusterengine to identify if the multiclusterengine is
		paused or not.
	*/
	AnnotationMCEPause           = "installer.multicluster.openshift.io/pause"
	DeprecatedAnnotationMCEPause = "pause"

	/*
		AnnotationReleaseVersion is an annotation used to indicate the release version that should be applied to all
		resources managed by the backplane operator.
	*/
	AnnotationReleaseVersion = "installer.multicluster.openshift.io/release-version"

	/*
		AnnotationTemplateOverridesCM is an annotation used in multiclusterengine to specify a custom ConfigMap
		containing resource template overrides.
	*/
	AnnotationTemplateOverridesCM = "installer.multicluster.openshift.io/template-override-configmap"

	/*
		AnnotationHubSize is an annotation used in multiclusterhub to specify a hub size that can be
		used by other components. MCE can use this standalone or inherit it from MCH
	*/
	AnnotationHubSize = "installer.multicluster.openshift.io/hub-size"
)

/*
IsPaused checks if the MultiClusterHub instance is labeled as paused.
It returns true if the instance is paused, otherwise false.
*/
func IsPaused(instance *backplanev1.MultiClusterEngine) bool {
	return IsAnnotationTrue(instance, AnnotationMCEPause) || IsAnnotationTrue(instance, DeprecatedAnnotationMCEPause)
}

/*
GetHubSize gets the current hubsize, returning "Small" as default if the annotation is not found.
*/
func GetHubSize(instance *backplanev1.MultiClusterEngine) backplanev1.HubSize {
	hubsize := getAnnotation(instance, AnnotationHubSize)
	if hubsize != "" {
		return backplanev1.HubSize(hubsize)
	}
	return backplanev1.Small
}

/*
IsAnnotationTrue checks if a specific annotation key in the given instance is set to "true".
*/
func IsAnnotationTrue(instance *backplanev1.MultiClusterEngine, annotationKey string) bool {
	a := instance.GetAnnotations()
	if a == nil {
		return false
	}

	value := strings.EqualFold(a[annotationKey], "true")
	return value
}

/*
AnnotationsMatch checks if all specified annotations in the 'old' map match the corresponding ones in the 'new' map.
It returns true if all annotations match, otherwise false.
*/
func AnnotationsMatch(old, new map[string]string) bool {
	return getAnnotationOrDefaultForMap(old, new, AnnotationMCEPause, DeprecatedAnnotationMCEPause) &&
		getAnnotationOrDefaultForMap(old, new, AnnotationImageRepo, DeprecatedAnnotationImageRepo) &&
		getAnnotationOrDefaultForMap(old, new, AnnotationImageOverridesCM, DeprecatedAnnotationImageOverridesCM) &&
		getAnnotationOrDefaultForMap(old, new, AnnotationKubeconfig, DeprecatedAnnotationKubeconfig) &&
		getAnnotationOrDefaultForMap(old, new, AnnotationTemplateOverridesCM, "")
}

// AnnotationPresent returns true if annotation is present on object
func AnnotationPresent(annotation string, obj client.Object) bool {
	if obj.GetAnnotations() == nil {
		return false
	}
	_, exists := obj.GetAnnotations()[annotation]
	return exists
}

/*
GetAnnotation returns the annotation value for a given key from the instance's annotations,
or an empty string if the annotation is not set.
*/
func getAnnotation(instance *backplanev1.MultiClusterEngine, key string) string {
	a := instance.GetAnnotations()
	if a == nil {
		return ""
	}
	return a[key]
}

/*
getAnnotationOrDefault retrieves the value of the primary annotation key,
falling back to the deprecated key if the primary key is not set.
*/
func getAnnotationOrDefault(instance *backplanev1.MultiClusterEngine, primaryKey, deprecatedKey string) string {
	primaryValue := getAnnotation(instance, primaryKey)
	if primaryValue != "" {
		return primaryValue
	}

	return getAnnotation(instance, deprecatedKey)
}

/*
getAnnotationOrDefaultForMap checks if the annotation value from the 'old' map matches the one from the 'new' map,
including deprecated annotations.
*/
func getAnnotationOrDefaultForMap(old, new map[string]string, primaryKey, deprecatedKey string) bool {
	oldValue := old[primaryKey]

	if oldValue == "" {
		oldValue = old[deprecatedKey]
	}

	newValue := new[primaryKey]
	if newValue == "" {
		newValue = new[deprecatedKey]
	}

	return oldValue == newValue
}

/*
GetHostedCredentialsSecret returns the NamespacedName of the secret containing the kubeconfig
to access the hosted cluster, using the primary annotation key and falling back to the deprecated key if not set.
*/
func GetHostedCredentialsSecret(mce *backplanev1.MultiClusterEngine) (types.NamespacedName, error) {
	nn := types.NamespacedName{}
	nn.Name = getAnnotationOrDefault(mce, AnnotationKubeconfig, DeprecatedAnnotationKubeconfig)

	if nn.Name == "" {
		return nn, fmt.Errorf("no kubeconfig secret annotation defined in %s", mce.Name)
	}

	nn.Namespace = mce.Spec.TargetNamespace
	if mce.Spec.TargetNamespace == "" {
		nn.Namespace = backplanev1.DefaultTargetNamespace
	}
	return nn, nil
}

/*
GetImageRepository returns the image repository annotation value,
using the primary annotation key and falling back to the deprecated key if not set.
*/
func GetImageRepository(instance *backplanev1.MultiClusterEngine) string {
	return getAnnotationOrDefault(instance, AnnotationImageRepo, DeprecatedAnnotationImageRepo)
}

/*
GetImageOverridesConfigmapName returns the image overrides ConfigMap annotation value,
using the primary annotation key and falling back to the deprecated key if not set.
*/
func GetImageOverridesConfigmapName(instance *backplanev1.MultiClusterEngine) string {
	return getAnnotationOrDefault(instance, AnnotationImageOverridesCM, DeprecatedAnnotationImageOverridesCM)
}

/*
GetTemplateOverridesConfigmapName returns the template overrides ConfigMap annotation value,
or an empty string if not set.
*/
func GetTemplateOverridesConfigmapName(instance *backplanev1.MultiClusterEngine) string {
	return getAnnotation(instance, AnnotationTemplateOverridesCM)
}

/*
IsAnnotationTrue checks if a specific annotation key in the given instance is set to "true".
*/
func IsTemplateAnnotationTrue(instance *unstructured.Unstructured, annotationKey string) bool {
	a := instance.GetAnnotations()
	if a == nil {
		return false
	}

	value := strings.EqualFold(a[annotationKey], "true")
	return value
}

/*
HasAnnotation checks if a specific annotation key exists in the instance's annotations.
*/
func HasAnnotation(instance *backplanev1.MultiClusterEngine, annotationKey string) bool {
	a := instance.GetAnnotations()
	if a == nil {
		return false
	}

	_, exists := a[annotationKey]
	return exists
}

func GetEdgeManagerEnabled(instance *backplanev1.MultiClusterEngine) string {
	if HasAnnotation(instance, AnnotationEdgeManagerEnabled) {
		return instance.GetAnnotations()[AnnotationEdgeManagerEnabled]
	} else {
		return "false"
	}

}

/*
OverrideImageRepository modifies image references in a map to use a specified image repository.
*/
func OverrideImageRepository(imageOverrides map[string]string, imageRepo string) map[string]string {
	for imageKey, imageRef := range imageOverrides {
		image := strings.LastIndex(imageRef, "/")
		imageOverrides[imageKey] = fmt.Sprintf("%s%s", imageRepo, imageRef[image:])
	}
	return imageOverrides
}

/*
ShouldIgnoreOCPVersion checks if the instance is annotated to skip the minimum OCP version requirement.
*/
func ShouldIgnoreOCPVersion(instance *backplanev1.MultiClusterEngine) bool {
	return HasAnnotation(instance, AnnotationIgnoreOCPVersion) ||
		HasAnnotation(instance, DeprecatedAnnotationIgnoreOCPVersion)
}
