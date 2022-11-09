// Copyright Contributors to the Open Cluster Management project

package foundation

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/utils"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ocmapiv1 "open-cluster-management.io/api/operator/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func hostedName(m *v1.MultiClusterEngine) string {
	return fmt.Sprintf("%s-cluster-manager", m.Name)
}

// HostedClusterManager returns the ClusterManager in hosted mode
func HostedClusterManager(m *v1.MultiClusterEngine, overrides map[string]string) *unstructured.Unstructured {
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
			Name: hostedName(m),
		},
		Spec: ocmapiv1.ClusterManagerSpec{
			RegistrationImagePullSpec: RegistrationImage(overrides),
			WorkImagePullSpec:         WorkImage(overrides),
			PlacementImagePullSpec:    PlacementImage(overrides),
			NodePlacement: ocmapiv1.NodePlacement{
				NodeSelector: m.Spec.NodeSelector,
				Tolerations:  cmTolerations,
			},
			DeployOption: ocmapiv1.ClusterManagerDeployOption{
				Mode: ocmapiv1.InstallModeHosted,
				Hosted: &ocmapiv1.HostedClusterManagerConfiguration{
					RegistrationWebhookConfiguration: ocmapiv1.WebhookConfiguration{
						Address: "192.168.218.1",
						Port:    443,
					},
					WorkWebhookConfiguration: ocmapiv1.WebhookConfiguration{
						Address: "192.168.218.2",
						Port:    443,
					},
				},
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
