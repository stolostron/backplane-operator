// Copyright Contributors to the Open Cluster Management project

package foundation

import (
	"context"
	"os"
	"path"
	"path/filepath"

	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	apixv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	v1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/utils"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	ocmapiv1 "open-cluster-management.io/api/operator/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// RegistrationImageKey used by registration deployments
	RegistrationImageKey = "registration"
	// WorkImageKey used by work deployments
	WorkImageKey = "work"
	// PlacementImageKey used by placement deployments
	PlacementImageKey = "placement"
	// AddonManagerImageKey used by addon-manager deployments
	AddonManagerImageKey          = "addon-manager"
	addonPath                     = "pkg/templates/clustermanagementaddons/"
	clusterManagementAddonCRDName = "clustermanagementaddons.addon.open-cluster-management.io"
	ClusterManagementAddonKind    = "ClusterManagementAddOn"
)

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

// AddonManagerImage ...
func AddonManagerImage(overrides map[string]string) string {
	return overrides[AddonManagerImageKey]
}

func ClusterManager(m *v1.MultiClusterEngine, overrides map[string]string) *unstructured.Unstructured {
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
			AddOnManagerImagePullSpec: AddonManagerImage(overrides),
			NodePlacement: ocmapiv1.NodePlacement{
				NodeSelector: m.Spec.NodeSelector,
				Tolerations:  cmTolerations,
			},
			DeployOption: ocmapiv1.ClusterManagerDeployOption{
				Mode: ocmapiv1.InstallModeDefault,
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

// CanInstallAddons returns true if addons can be installed
func CanInstallAddons(ctx context.Context, client client.Client) bool {
	addonCRD := &apixv1.CustomResourceDefinition{}
	err := client.Get(ctx, types.NamespacedName{Name: clusterManagementAddonCRDName}, addonCRD)
	return err == nil
}

func GetAddons() ([]*unstructured.Unstructured, error) {
	var addons []*unstructured.Unstructured

	addonPath := addonPath
	if val, ok := os.LookupEnv("DIRECTORY_OVERRIDE"); ok {
		addonPath = path.Join(val, addonPath)
	}

	err := filepath.Walk(addonPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		addon := &unstructured.Unstructured{}
		if info == nil || info.IsDir() {
			return nil
		}
		bytesFile, e := os.ReadFile(path)
		if e != nil {
			return err
		}
		if err = yaml.Unmarshal(bytesFile, addon); err != nil {
			return err
		}
		addons = append(addons, addon)
		return nil
	})
	return addons, err

}
