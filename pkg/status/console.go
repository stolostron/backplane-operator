// Copyright Contributors to the Open Cluster Management project
package status

import (
	"context"
	"fmt"

	bpv1 "github.com/stolostron/backplane-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeploymentStatus fulfills the StatusReporter interface for deployments
type ConsoleUnavailableStatus struct {
	types.NamespacedName
}

func (cs ConsoleUnavailableStatus) GetName() string {
	return cs.Name
}

func (cs ConsoleUnavailableStatus) GetNamespace() string {
	return cs.Namespace
}

func (cms ConsoleUnavailableStatus) GetKind() string {
	return "Deployment"
}

// Converts a deployment's status to a backplane component status
func (cs ConsoleUnavailableStatus) Status(k8sClient client.Client) bpv1.ComponentCondition {
	deploy := &appsv1.Deployment{}
	err := k8sClient.Get(context.TODO(), cs.NamespacedName, deploy)
	if err != nil && !apierrors.IsNotFound(err) {
		fmt.Println("Err getting deployment", err)
		return unknownStatus(cs.GetName(), cs.GetKind())
	}
	return mapConsoleDeployment(cs.GetName())
}

func mapConsoleDeployment(name string) bpv1.ComponentCondition {

	ret := bpv1.ComponentCondition{
		Name:               name,
		Kind:               "Deployment",
		Type:               string(appsv1.DeploymentAvailable),
		Status:             metav1.ConditionFalse,
		LastUpdateTime:     metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             "OCP Console missing",
		Message:            "The OCP Console must be enabled before using ACM Console",
		Available:          true,
	}

	return ret
}
