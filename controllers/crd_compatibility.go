// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	clusterAPIGroupVersion = "operator.openshift.io/v1alpha1"
	clusterAPIKind         = "ClusterAPI"
	clusterAPIConfigName   = "cluster"
	capiCRDSuffix          = ".cluster.x-k8s.io"
	ccapioSyncTimeout      = 5 * time.Minute
	ccapioPollInterval     = 2 * time.Second
)

type groupVersionDiscoverer interface {
	ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error)
}

func isClusterAPIRegistered(discoveryClient groupVersionDiscoverer) (bool, error) {
	apis, err := discoveryClient.ServerResourcesForGroupVersion(clusterAPIGroupVersion)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to discover API resources for %s: %w", clusterAPIGroupVersion, err)
	}
	if apis != nil {
		for _, api := range apis.APIResources {
			if api.Kind == clusterAPIKind {
				return true, nil
			}
		}
	}
	return false, nil
}

func isCAPICRD(name string) bool {
	return strings.HasSuffix(name, capiCRDSuffix)
}

func getCAPICRDNames(crds []*unstructured.Unstructured) []string {
	var names []string
	seen := make(map[string]bool)
	for _, crd := range crds {
		name := crd.GetName()
		if isCAPICRD(name) && !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	slices.Sort(names)
	return names
}

func ensureUnmanagedCRDs(ctx context.Context, c client.Client, crds []*unstructured.Unstructured) (int64, error) {
	capiCRDNames := getCAPICRDNames(crds)
	if len(capiCRDNames) == 0 {
		return 0, nil
	}

	patchData, err := json.Marshal(map[string]any{
		"apiVersion": clusterAPIGroupVersion,
		"kind":       clusterAPIKind,
		"metadata": map[string]any{
			"name": clusterAPIConfigName,
		},
		"spec": map[string]any{
			"unmanagedCustomResourceDefinitions": capiCRDNames,
		},
	})
	if err != nil {
		return 0, fmt.Errorf("failed to marshal ClusterAPI config: %w", err)
	}

	clusterAPI := &unstructured.Unstructured{}
	clusterAPI.SetGroupVersionKind(clusterAPIGVK())
	clusterAPI.SetName(clusterAPIConfigName)

	if err := c.Patch(ctx, clusterAPI, client.RawPatch(types.ApplyPatchType, patchData),
		client.ForceOwnership, client.FieldOwner("backplane-operator"),
	); err != nil {
		return 0, fmt.Errorf("failed to apply ClusterAPI config: %w", err)
	}
	return clusterAPI.GetGeneration(), nil
}

func waitForCAPIOperatorSync(ctx context.Context, c client.Client, generation int64) error {
	waitCtx, cancel := context.WithTimeout(ctx, ccapioSyncTimeout)
	defer cancel()

	return wait.PollUntilContextCancel(waitCtx, ccapioPollInterval, true, func(ctx context.Context) (bool, error) {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(clusterAPIGVK())
		if err := c.Get(ctx, client.ObjectKey{Name: clusterAPIConfigName}, obj); err != nil {
			return false, fmt.Errorf("failed to get ClusterAPI config: %w", err)
		}

		status, found, _ := unstructured.NestedMap(obj.Object, "status")
		if !found {
			return false, nil
		}

		observedGen, _, _ := unstructured.NestedInt64(status, "observedRevisionGeneration")
		if observedGen < generation {
			return false, nil
		}

		currentRev, _, _ := unstructured.NestedString(status, "currentRevision")
		desiredRev, _, _ := unstructured.NestedString(status, "desiredRevision")
		if currentRev == "" || currentRev != desiredRev {
			return false, nil
		}

		return true, nil
	})
}

func clusterAPIGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "operator.openshift.io",
		Version: "v1alpha1",
		Kind:    clusterAPIKind,
	}
}
