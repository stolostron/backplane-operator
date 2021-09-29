// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"fmt"
	"strings"

	backplanev1alpha1 "github.com/open-cluster-management/backplane-operator/api/v1alpha1"
)

var (
	// AnnotationMCEPause sits in multiclusterengine annotations to identify if the multiclusterengine is paused or not
	AnnotationMCEPause = "pause"
	// AnnotationImageRepo sits in multiclusterengine annotations to identify a custom image repository to use
	AnnotationImageRepo = "imageRepository"
)

// IsPaused returns true if the multiclusterengine instance is labeled as paused, and false otherwise
func IsPaused(instance *backplanev1alpha1.MultiClusterEngine) bool {
	a := instance.GetAnnotations()
	if a == nil {
		return false
	}

	if a[AnnotationMCEPause] != "" && strings.EqualFold(a[AnnotationMCEPause], "true") {
		return true
	}

	return false
}

// AnnotationsMatch returns true if all annotation values used by the operator match
func AnnotationsMatch(old, new map[string]string) bool {
	return old[AnnotationMCEPause] == new[AnnotationMCEPause] &&
		old[AnnotationImageRepo] == new[AnnotationImageRepo]
}

// getAnnotation returns the annotation value for a given key, or an empty string if not set
func getAnnotation(instance *backplanev1alpha1.MultiClusterEngine, key string) string {
	a := instance.GetAnnotations()
	if a == nil {
		return ""
	}
	return a[key]
}

// GetImageRepository returns the image repo annotation, or an empty string if not set
func GetImageRepository(instance *backplanev1alpha1.MultiClusterEngine) string {
	return getAnnotation(instance, AnnotationImageRepo)
}

func OverrideImageRepository(imageOverrides map[string]string, imageRepo string) map[string]string {
	for imageKey, imageRef := range imageOverrides {
		image := strings.LastIndex(imageRef, "/")
		imageOverrides[imageKey] = fmt.Sprintf("%s%s", imageRepo, imageRef[image:])
	}
	return imageOverrides
}
