// Copyright Contributors to the Open Cluster Management project
package status

import (
	"context"
	"fmt"

	bpv1alpha1 "github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeploymentStatus fulfills the StatusReporter interface for deployments
type DeploymentStatus struct {
	types.NamespacedName
}

func (ds DeploymentStatus) GetName() string {
	return ds.Name
}

func (ds DeploymentStatus) GetNamespace() string {
	return ds.Namespace
}

func (ds DeploymentStatus) GetKind() string {
	return "Deployment"
}

// Converts a deployment's status to a backplane component status
func (ds DeploymentStatus) Status(k8sClient client.Client) bpv1alpha1.ComponentCondition {
	deploy := &appsv1.Deployment{}
	err := k8sClient.Get(context.TODO(), ds.NamespacedName, deploy)
	if err != nil && !apierrors.IsNotFound(err) {
		fmt.Println("Err getting deployment", err)
		return unknownStatus(ds.GetName(), ds.GetKind())
	} else if apierrors.IsNotFound(err) {
		return unknownStatus(ds.GetName(), ds.GetKind())
	}

	return mapDeployment(deploy)
}

func mapDeployment(ds *appsv1.Deployment) bpv1alpha1.ComponentCondition {
	if len(ds.Status.Conditions) < 1 {
		return unknownStatus(ds.Name, ds.Kind)
	}

	dcs := latestDeployCondition(ds.Status.Conditions)
	ret := bpv1alpha1.ComponentCondition{
		Name:               ds.Name,
		Kind:               "Deployment",
		Type:               string(dcs.Type),
		Status:             metav1.ConditionStatus(string(dcs.Status)),
		LastUpdateTime:     dcs.LastUpdateTime,
		LastTransitionTime: dcs.LastTransitionTime,
		Reason:             dcs.Reason,
		Message:            dcs.Message,
	}
	if successfulDeploy(ds) {
		ret.Available = true
		ret.Message = ""
	}

	// Because our definition of success is different than the deployment's it is possible we indicate failure
	// despite an available deployment present. To avoid confusion we should show a different status.
	if dcs.Type == appsv1.DeploymentAvailable && dcs.Status == corev1.ConditionTrue && ret.Available == false {
		sub := progressingDeployCondition(ds.Status.Conditions)
		ret = bpv1alpha1.ComponentCondition{
			Name:               ds.Name,
			Kind:               "Deployment",
			Type:               string(sub.Type),
			Status:             metav1.ConditionStatus(string(sub.Status)),
			LastUpdateTime:     sub.LastUpdateTime,
			LastTransitionTime: sub.LastTransitionTime,
			Reason:             sub.Reason,
			Message:            sub.Message,
			Available:          false,
		}
	}

	return ret
}

func latestDeployCondition(conditions []appsv1.DeploymentCondition) appsv1.DeploymentCondition {
	if len(conditions) < 1 {
		return appsv1.DeploymentCondition{}
	}
	latest := conditions[0]
	for i := range conditions {
		if conditions[i].LastTransitionTime.Time.After(latest.LastTransitionTime.Time) {
			latest = conditions[i]
		}
	}
	return latest
}

func progressingDeployCondition(conditions []appsv1.DeploymentCondition) appsv1.DeploymentCondition {
	progressing := appsv1.DeploymentCondition{}
	for i := range conditions {
		if conditions[i].Type == appsv1.DeploymentProgressing {
			progressing = conditions[i]
		}
	}
	return progressing
}

func successfulDeploy(d *appsv1.Deployment) bool {
	for _, c := range d.Status.Conditions {
		if c.Type == appsv1.DeploymentAvailable && c.Status == corev1.ConditionFalse {
			return false
		}
	}

	if d.Status.UnavailableReplicas > 0 {
		return false
	}

	return true
	// latest := latestDeployCondition(d.Status.Conditions)
}
