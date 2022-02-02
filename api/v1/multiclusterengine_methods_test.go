// Copyright Contributors to the Open Cluster Management project
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func Test_ComponentEnabled(t *testing.T) {
	// tracker := StatusTracker{Client: fake.NewClientBuilder().Build()}

	t.Run("No components enabled", func(t *testing.T) {
		mce := MultiClusterEngine{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "multicluster.openshift.io/v1",
				Kind:       "MultiClusterEngine",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
			Spec: MultiClusterEngineSpec{
				TargetNamespace: "test",
			},
		}

		msaEnabled := mce.ComponentEnabled(ManagedServiceAccount)
		if msaEnabled {
			t.Fatal("Expected no component enabled, but ManagedServiceAccount enabled")
		}
		// FUTURE: INCLUDE ALL OTHER COMPONENT ENABLED OPTIONS HERE, ONCE THEY EXIST
	})

	t.Run("ManagedServiceAccount not enabled", func(t *testing.T) {
		mce := MultiClusterEngine{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "multicluster.openshift.io/v1",
				Kind:       "MultiClusterEngine",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
			Spec: MultiClusterEngineSpec{
				TargetNamespace: "test",
			},
		}

		msaEnabled := mce.ComponentEnabled(ManagedServiceAccount)
		if msaEnabled {
			t.Fatal("Expected ManagedServiceAccount to not be enabled (no ComponentConfig)")
		}

		mce = MultiClusterEngine{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "multicluster.openshift.io/v1",
				Kind:       "MultiClusterEngine",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
			Spec: MultiClusterEngineSpec{
				TargetNamespace: "test",
				ComponentConfig: &ComponentConfig{},
			},
		}

		msaEnabled = mce.ComponentEnabled(ManagedServiceAccount)
		if msaEnabled {
			t.Fatal("Expected ManagedServiceAccount to not be enabled (empty ComponentConfig)")
		}

		mce = MultiClusterEngine{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "multicluster.openshift.io/v1",
				Kind:       "MultiClusterEngine",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
			Spec: MultiClusterEngineSpec{
				TargetNamespace: "test",
				ComponentConfig: &ComponentConfig{
					ManagedServiceAccount: &ManagedServiceAccountConfig{},
				},
			},
		}

		msaEnabled = mce.ComponentEnabled(ManagedServiceAccount)
		if msaEnabled {
			t.Fatal("Expected ManagedServiceAccount to not be enabled (empty ManagedServiceAccountConfig)")
		}

		mce = MultiClusterEngine{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "multicluster.openshift.io/v1",
				Kind:       "MultiClusterEngine",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
			Spec: MultiClusterEngineSpec{
				TargetNamespace: "test",
				ComponentConfig: &ComponentConfig{
					ManagedServiceAccount: &ManagedServiceAccountConfig{
						Enable: false,
					},
				},
			},
		}

		msaEnabled = mce.ComponentEnabled(ManagedServiceAccount)
		if msaEnabled {
			t.Fatal("Expected ManagedServiceAccount to not be enabled (Enable: false)")
		}
	})

	t.Run("ManagedServiceAccount enabled", func(t *testing.T) {
		mce := MultiClusterEngine{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "multicluster.openshift.io/v1",
				Kind:       "MultiClusterEngine",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
			Spec: MultiClusterEngineSpec{
				TargetNamespace: "test",
				ComponentConfig: &ComponentConfig{
					ManagedServiceAccount: &ManagedServiceAccountConfig{
						Enable: false,
					},
				},
			},
		}

		msaEnabled := mce.ComponentEnabled(ManagedServiceAccount)
		if !msaEnabled {
			t.Fatal("Expected ManagedServiceAccount to be enabled")
		}
		// FUTURE: INCLUDE ALL OTHER COMPONENT ENABLED OPTIONS HERE, ONCE THEY EXIST
	})
}
