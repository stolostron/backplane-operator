// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"fmt"
	"strings"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// AnnotationMCEPause sits in multiclusterengine annotations to identify if the multiclusterengine is paused or not
	AnnotationMCEPause = "pause"
	// AnnotationMCEIgnore labels a resource as something the operator should ignore and not update
	AnnotationMCEIgnore = "multiclusterengine.openshift.io/ignore"
	// AnnotationIgnoreOCPVersion indicates the operator should not check the OCP version before proceeding when set
	AnnotationIgnoreOCPVersion = "ignoreOCPVersion"
	// AnnotationImageRepo sits in multiclusterengine annotations to identify a custom image repository to use
	AnnotationImageRepo = "imageRepository"
	// AnnotationImageOverridesCM identifies a configmap name containing an image override mapping
	AnnotationImageOverridesCM = "imageOverridesCM"
	// AnnotationKubeconfig is the secret name residing in targetcontaining the kubeconfig to access the remote cluster
	AnnotationKubeconfig = "mce-kubeconfig"

	AnnotationHostedClusterName      = "hosted-cluster-name"
	AnnotationHostedClusterNamespace = "hosted-cluster-namespace"
	AnnotationHyperShiftNamespace    = "hyper-shift-control-plane-namespace"
)

// IsPaused returns true if the multiclusterengine instance is labeled as paused, and false otherwise
func IsPaused(instance *backplanev1.MultiClusterEngine) bool {
	a := instance.GetAnnotations()
	if a == nil {
		return false
	}

	if a[AnnotationMCEPause] != "" && strings.EqualFold(a[AnnotationMCEPause], "true") {
		return true
	}

	return false
}

// ShouldIgnoreOCPVersion returns true if the multiclusterengine instance is annotated to skip
// the minimum OCP version requirement
func ShouldIgnoreOCPVersion(instance *backplanev1.MultiClusterEngine) bool {
	a := instance.GetAnnotations()
	if a == nil {
		return false
	}

	if _, ok := a[AnnotationIgnoreOCPVersion]; ok {
		return true
	}
	return false
}

// AnnotationsMatch returns true if all annotation values used by the operator match
func AnnotationsMatch(old, new map[string]string) bool {
	return old[AnnotationMCEPause] == new[AnnotationMCEPause] &&
		old[AnnotationImageRepo] == new[AnnotationImageRepo]
}

// AnnotationPresent returns true if annotation is present on object
func AnnotationPresent(annotation string, obj client.Object) bool {
	if obj.GetAnnotations() == nil {
		return false
	}
	_, exists := obj.GetAnnotations()[annotation]
	return exists
}

// getAnnotation returns the annotation value for a given key, or an empty string if not set
func getAnnotation(instance *backplanev1.MultiClusterEngine, key string) string {
	a := instance.GetAnnotations()
	if a == nil {
		return ""
	}
	return a[key]
}

// GetImageRepository returns the image repo annotation, or an empty string if not set
func GetImageRepository(instance *backplanev1.MultiClusterEngine) string {
	return getAnnotation(instance, AnnotationImageRepo)
}

func OverrideImageRepository(imageOverrides map[string]string, imageRepo string) map[string]string {
	for imageKey, imageRef := range imageOverrides {
		image := strings.LastIndex(imageRef, "/")
		imageOverrides[imageKey] = fmt.Sprintf("%s%s", imageRepo, imageRef[image:])
	}
	return imageOverrides
}

// GetImageOverridesConfigmap returns the images override configmap annotation, or an empty string if not set
func GetImageOverridesConfigmap(instance *backplanev1.MultiClusterEngine) string {
	return getAnnotation(instance, AnnotationImageOverridesCM)
}

func GetHostedCredentialsSecret(mce *backplanev1.MultiClusterEngine) (types.NamespacedName, error) {
	nn := types.NamespacedName{}
	if mce.Annotations == nil || mce.Annotations[AnnotationKubeconfig] == "" {
		return nn, fmt.Errorf("no kubeconfig secret annotation defined in %s", mce.Name)
	}
	nn.Name = mce.Annotations[AnnotationKubeconfig]

	nn.Namespace = mce.Spec.TargetNamespace
	if mce.Spec.TargetNamespace == "" {
		nn.Namespace = backplanev1.DefaultTargetNamespace
	}
	return nn, nil
}

func GetKonnectivitySecret(mce *backplanev1.MultiClusterEngine) (types.NamespacedName, error) {
	nn := types.NamespacedName{}
	if mce.Annotations == nil || mce.Annotations[AnnotationHyperShiftNamespace] == "" {
		return nn, fmt.Errorf("no konnectivity secret annotation defined in %s", mce.Name)
	}
	nn.Name = "konnectivity-agent"

	nn.Namespace = mce.Annotations[AnnotationHyperShiftNamespace]
	return nn, nil
}

func GetExternalKubeconfigSecret(mce *backplanev1.MultiClusterEngine) (types.NamespacedName, error) {
	nn := types.NamespacedName{}
	if mce.Annotations == nil || mce.Annotations[AnnotationHostedClusterNamespace] == "" || mce.Annotations[AnnotationHostedClusterName] == "" {
		return nn, fmt.Errorf("no hosted cluster secret annotation defined in %s", mce.Name)
	}
	nn.Name = fmt.Sprintf("%s-admin-kubeconfig", mce.Annotations[AnnotationHostedClusterName])

	nn.Namespace = mce.Annotations[AnnotationHostedClusterNamespace]
	return nn, nil
}

func GetHostedClusterNamespace(mce *backplanev1.MultiClusterEngine) (string, error) {
	var ns string
	if mce.Annotations == nil || mce.Annotations[AnnotationHostedClusterNamespace] == "" {
		return ns, fmt.Errorf("no hosted cluster namespace annotation defined in %s", mce.Name)
	}

	ns = mce.Annotations[AnnotationHostedClusterNamespace]
	return ns, nil
}
func GetHyperShiftNamespace(mce *backplanev1.MultiClusterEngine) (string, error) {
	var ns string
	if mce.Annotations == nil || mce.Annotations[AnnotationHyperShiftNamespace] == "" {
		return ns, fmt.Errorf("no hyper shift conntrol plane namespace annotation defined in %s", mce.Name)
	}

	ns = mce.Annotations[AnnotationHyperShiftNamespace]
	return ns, nil
}
func GetHostedClusterName(mce *backplanev1.MultiClusterEngine) (string, error) {
	var ns string
	if mce.Annotations == nil || mce.Annotations[AnnotationHostedClusterName] == "" {
		return ns, fmt.Errorf("no hosted cluster name annotation defined in %s", mce.Name)
	}

	ns = mce.Annotations[AnnotationHostedClusterName]
	return ns, nil
}
