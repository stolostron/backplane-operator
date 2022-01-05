// Copyright Contributors to the Open Cluster Management project

package foundation

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/stolostron/backplane-operator/api/v1alpha1"
	"github.com/stolostron/backplane-operator/pkg/utils"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ocmapiv1 "open-cluster-management.io/api/operator/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RegistrationImageKey used by registration deployments
const RegistrationImageKey = "registration"

// WorkImageKey used by work deployments
const WorkImageKey = "work"

// PlacementImageKey used by placement deployments
const PlacementImageKey = "placement"

// RegistrationImage ...
func RegistrationImage(overrides map[string]string) string {
	return overrides[RegistrationImageKey]
}

// WorkImage ...
func WorkImage(overrides map[string]string) string {
	return overrides[WorkImageKey]
}

// PlacementImage ...
func PlacementImage(overrides map[string]string) string {
	return overrides[PlacementImageKey]
}

func ClusterManager(m *v1alpha1.MultiClusterEngine, overrides map[string]string) *unstructured.Unstructured {
	log := log.FromContext(context.Background())

	cmTolerations := []corev1.Toleration{}
	if m.Spec.Tolerations != nil {
		cmTolerations = m.Spec.Tolerations
	} else {
		cmTolerations = utils.DefaultTolerations()
	}

	cm := &ocmapiv1.ClusterManager{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "operator.open-cluster-management.io/v1",
			Kind:       "ClusterManager",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-manager",
		},
		Spec: ocmapiv1.ClusterManagerSpec{
			RegistrationImagePullSpec: RegistrationImage(overrides),
			WorkImagePullSpec:         WorkImage(overrides),
			PlacementImagePullSpec:    PlacementImage(overrides),
			NodePlacement: ocmapiv1.NodePlacement{
				NodeSelector: m.Spec.NodeSelector,
				Tolerations:  cmTolerations,
			},
		},
	}

	utils.AddBackplaneConfigLabels(cm, m.GetName())
	unstructured, err := utils.CoreToUnstructured(cm)
	if err != nil {
		log.Error(err, err.Error())
	}

	return unstructured
}
