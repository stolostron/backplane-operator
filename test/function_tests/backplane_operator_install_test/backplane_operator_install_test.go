// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package backplane_install_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/Masterminds/semver"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	backplane "github.com/stolostron/backplane-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/apimachinery/pkg/types"
)

const (
	multiClusterEngineName = "backplane"
	// installTimeout is the max time an install should take
	installTimeout = time.Minute * 5
	// deleteTimeout is the max time a delete should take
	deleteTimeout = time.Minute * 3
	listTimeout   = time.Second * 30
	duration      = time.Second * 1
	interval      = time.Millisecond * 250
)

var (
	k8sClient client.Client

	multiClusterEngine = types.NamespacedName{
		Name: multiClusterEngineName,
	}
)

var _ = Describe("MultiClusterEngine Test Suite", func() {
	Context("empty spec", func() {
		mce := defaultmultiClusterEngine()
		fullTestSuite(mce)
	})

	Context("target existing namespace", func() {
		mce := defaultmultiClusterEngine()
		mce.Spec.TargetNamespace = "existing-ns"

		It("Should create a namespace to install to", func() {
			err := k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "existing-ns"},
			})
			if err != nil {
				Expect(apierrors.IsAlreadyExists(err)).To(BeTrue())
			}
		})

		fullTestSuite(mce)

		It("should preserve namespace", func() {
			createdNS := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "existing-ns"}, createdNS)).To(Succeed())
			Expect(k8sClient.Delete(ctx, createdNS)).Should(Succeed())
		})
	})

	Context("target new namespace", func() {
		mce := defaultmultiClusterEngine()
		mce.Spec.TargetNamespace = "new-ns"

		fullTestSuite(mce)

		It("should remove namespace", func() {
			Eventually(func(g Gomega) {
				createdNS := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "new-ns"}, createdNS)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Expected IsNotFound error, got error:", err)
			}, time.Minute, interval).Should(Succeed())
		})
	})

})

// complete set of MCE tests
var fullTestSuite = func(mce *backplane.MultiClusterEngine) {
	It("Should install all components ", func() {
		By("By creating a new BackplaneConfig", func() {
			Expect(k8sClient.Create(ctx, mce)).Should(Succeed())
		})
	})

	Describe("Basic install", installTests())
	Describe("Webhook checks", webhookTests())
	Describe("Self-healing", selfHealingTests())
	Describe("Configuration options", configurationTests())

	It("Should remove mce ", func() {
		By("By deleting BackplaneConfig", func() {
			err := k8sClient.Delete(ctx, mce)
			if err != nil {
				Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Error calling delete on MCE", err.Error())
			}
		})
	})

	Describe("Uninstallation", uninstallTests())
}

var validateMCEConsoleTests = func(existingMCE *backplane.MultiClusterEngine) {
	clusterVersion := &configv1.ClusterVersion{}
	clusterVersionKey := types.NamespacedName{Name: "version"}
	Expect(k8sClient.Get(ctx, clusterVersionKey, clusterVersion)).To(Succeed())
	version := clusterVersion.Status.History[0].Version
	Expect(clusterVersion.Status.History[0].State).Should(Equal(configv1.CompletedUpdate), "Expected CompletedUpdate status in clusterVersion resource")
	mceConsoleConstraint, err := semver.NewConstraint(">= 4.10.0-0")
	Expect(err).To(BeNil(), "Error creating semver constraint")
	semverVersion, err := semver.NewVersion(version)
	Expect(err).To(BeNil(), "Error creating semver constraint")

	if mceConsoleConstraint.Check(semverVersion) {
		By("OCP 4.10+ cluster detected. Checking MCE Console is installed")
		components := existingMCE.Status.Components
		found := false
		for _, component := range components {
			if component.Name == "console-mce-console" {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(), "Expected MCE Console to be found in MultiClusterEngine status")
		console := &operatorv1.Console{}
		consoleKey := types.NamespacedName{Name: "cluster"}
		Expect(k8sClient.Get(ctx, consoleKey, console)).To(Succeed())
		By("Ensuring mce plugin is enabled in openshift console")
		pluginsList := console.Spec.Plugins
		Expect(contains(pluginsList, "mce")).To(BeTrue(), "Expected MCE plugin to be enabled in console resource")
	} else {
		By("OCP cluster below 4.10 detected. Ensuring MCE Console is not installed")
		components := existingMCE.Status.Components
		for _, component := range components {
			if component.Name == "console-mce-console" {
				Expect(component.Type).To(Equal("NotPresent"), "Expected MCE Console to not be present")
				break
			}
		}
	}
}

var installTests = func() func() {
	return func() {
		It("should become available", func() {
			Eventually(func() bool {
				key := &backplane.MultiClusterEngine{}
				Expect(k8sClient.Get(ctx, multiClusterEngine, key)).To(Succeed())
				return key.Status.Phase == backplane.MultiClusterEnginePhaseAvailable
			}, installTimeout, interval).Should(BeTrue())

		})

		It("should have a healthy status", func() {
			existingMCE := &backplane.MultiClusterEngine{}
			Expect(k8sClient.Get(ctx, multiClusterEngine, existingMCE)).To(Succeed())

			By("checking the phase", func() {
				Expect(existingMCE.Status.Phase).To(Equal(backplane.MultiClusterEnginePhaseAvailable))
			})
			By("checking the components", func() {
				Expect(len(existingMCE.Status.Components)).Should(BeNumerically(">=", 6), "Expected at least 6 components in status")
			})
			By("Validate MCE Console", func() {
				validateMCEConsoleTests(existingMCE)
			})
			By("checking the conditions", func() {
				available := backplane.MultiClusterEngineCondition{}
				for _, c := range existingMCE.Status.Conditions {
					if c.Type == backplane.MultiClusterEngineAvailable {
						available = c
					}
				}
				Expect(available.Status).To(Equal(metav1.ConditionTrue))
			})

		})
	}
}

var uninstallTests = func() func() {
	return func() {
		It("should clean up all components", func() {
			Eventually(func() bool {
				err := k8sClient.Get(ctx, multiClusterEngine, &backplane.MultiClusterEngine{})
				if err == nil {
					return false
				}
				return apierrors.IsNotFound(err)
			}, deleteTimeout, interval).Should(BeTrue(), "There was an issue cleaning up the backplane.")

			validateDelete()
		})
	}
}

// configurationTests test variations in mce spec options
var configurationTests = func() func() {
	return func() {
		Context("default spec", func() {
			var defaultAvailabilityConfig backplane.AvailabilityType
			var defaultNodeSelector map[string]string
			defaultPullSecret := ""
			defaultTolerations := []corev1.Toleration{
				{
					Effect:   "NoSchedule",
					Key:      "node-role.kubernetes.io/infra",
					Operator: "Exists",
				},
				{
					Effect:   "NoSchedule",
					Key:      "dedicated",
					Operator: "Exists",
				},
			}

			It("should propogate default configuration values", func() {
				key := &backplane.MultiClusterEngine{}
				Expect(k8sClient.Get(ctx, multiClusterEngine, key)).To(Succeed())

				validateResourceSpecs(key.Spec.TargetNamespace, defaultNodeSelector, defaultPullSecret, defaultTolerations, defaultAvailabilityConfig)
			})

		})

		Context("customized spec", func() {
			backplaneAvailabilityConfig := backplane.HABasic
			backplaneNodeSelector := map[string]string{"beta.kubernetes.io/os": "linux"}
			backplanePullSecret := "test"
			backplaneTolerations := []corev1.Toleration{
				{
					Key:      "dedicated",
					Operator: "Exists",
					Effect:   "NoSchedule",
				},
			}

			It("should propogate spec configuration changes", func() {
				key := &backplane.MultiClusterEngine{}

				// retry update due to conflicts
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, multiClusterEngine, key)).To(Succeed())
					key.Spec.NodeSelector = backplaneNodeSelector
					key.Spec.ImagePullSecret = backplanePullSecret
					key.Spec.Tolerations = backplaneTolerations
					key.Spec.AvailabilityConfig = backplaneAvailabilityConfig
					g.Expect(k8sClient.Update(ctx, key)).To(Succeed())
				}, 10*time.Second, time.Second).Should(Succeed())

				validateResourceSpecs(key.Spec.TargetNamespace, backplaneNodeSelector, backplanePullSecret, backplaneTolerations, backplaneAvailabilityConfig)
			})
		})

		Context("toggled components", func() {
			It("should disable discovery and enable hypershift", func() {
				key := &backplane.MultiClusterEngine{}

				// retry update due to conflicts
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, multiClusterEngine, key)).To(Succeed())
					key.Disable(backplane.Discovery)
					key.Enable(backplane.HyperShift)
					g.Expect(k8sClient.Update(ctx, key)).To(Succeed())
				}, 10*time.Second, time.Second).Should(Succeed())

				targetDeploy := &appsv1.Deployment{}
				Eventually(func(g Gomega) {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: "discovery-operator", Namespace: key.Spec.TargetNamespace}, targetDeploy)
					g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Expected IsNotFound error, got error:", err)
					err = k8sClient.Get(ctx, types.NamespacedName{Name: "hypershift-addon-manager", Namespace: key.Spec.TargetNamespace}, targetDeploy)
					g.Expect(err).To(BeNil(), "Expected no error, got error:", err)
					err = k8sClient.Get(ctx, types.NamespacedName{Name: "hypershift-deployment-controller", Namespace: key.Spec.TargetNamespace}, targetDeploy)
					g.Expect(err).To(BeNil(), "Expected no error, got error:", err)
				}, 10*time.Second, time.Second).Should(Succeed())
			})
		})

	}
}

// selfHealingTests test that changes to components are auto-corrected
var selfHealingTests = func() func() {
	return func() {
		It("modify the Hiveconfig", func() {
			hiveConfig := &unstructured.Unstructured{}
			hiveConfig.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "hive.openshift.io",
				Version: "v1",
				Kind:    "HiveConfig",
			})
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "hive"}, hiveConfig)).To(Succeed())
			unstructured.SetNestedField(hiveConfig.Object, "debug", "spec", "logLevel")
			Expect(k8sClient.Update(ctx, hiveConfig)).Should(Succeed())
		})

		It("Should ensure the Backplane is self-correcting", func() {
			key := &backplane.MultiClusterEngine{}
			Expect(k8sClient.Get(ctx, multiClusterEngine, key)).To(Succeed())

			By("Checking metadata is maintained but not overwritten", func() {
				By("Manipulating annotations and backplane labels in the ocm-controller deployment", func() {
					targetDeploy := &appsv1.Deployment{}

					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ocm-controller", Namespace: key.Spec.TargetNamespace}, targetDeploy)).To(Succeed())
						targetDeploy.SetLabels(map[string]string{})
						targetDeploy.SetAnnotations(map[string]string{"testannotation": "test"})
						g.Expect(k8sClient.Update(ctx, targetDeploy)).Should(Succeed())
					}, 10*time.Second, time.Second).Should(Succeed())
				})

				By("Checking backplane labels are added back and custom annotations are preserved", func() {
					Eventually(func(g Gomega) {
						deploy := &appsv1.Deployment{}
						g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ocm-controller", Namespace: key.Spec.TargetNamespace}, deploy)).To(Succeed())

						l := deploy.GetLabels()
						g.Expect(l["backplaneconfig.name"]).To(Equal(multiClusterEngine.Name), "Missing backplane label")

						a := deploy.GetAnnotations()
						g.Expect(a["testannotation"]).To(Equal("test"), "Test annotation may have been stripped out of deployment")
					}, 20*time.Second, interval).Should(Succeed())
				})
			})

			By("Checking spec changes are set to their expected values", func() {
				By("Manipulating the spec in the ocm-controller deployment", func() {
					targetDeploy := &appsv1.Deployment{}
					Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ocm-controller", Namespace: key.Spec.TargetNamespace}, targetDeploy)).To(Succeed())

					targetDeploy.Spec.Template.Spec.ServiceAccountName = "test-sa"
					targetDeploy.SetAnnotations(map[string]string{"testannotation": "test2"})
					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Update(ctx, targetDeploy)).Should(Succeed())
					}, 30*time.Second, time.Second).Should(Succeed())

				})

				By("Confirming the spec is reset", func() {
					Eventually(func(g Gomega) {
						deploy := &appsv1.Deployment{}
						g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ocm-controller", Namespace: key.Spec.TargetNamespace}, deploy)).To(Succeed())

						// annotation is used to verify the deployment has been updated
						a := deploy.GetAnnotations()
						g.Expect(a["testannotation"]).To(Equal("test2"), "Deployment may not have been updated")

						g.Expect(deploy.Spec.Template.Spec.ServiceAccountName).NotTo(Equal("test-sa"), "Deployment restart policy change was not reverted by operator")
					}, 20*time.Second, interval).Should(Succeed())
				})
			})
		})

		It("Should ensure the Hiveconfig modifications are preserved", func() {
			hiveConfig := &unstructured.Unstructured{}
			hiveConfig.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "hive.openshift.io",
				Version: "v1",
				Kind:    "HiveConfig",
			})
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "hive"}, hiveConfig)).To(Succeed())
			logLevel, found, err := unstructured.NestedString(hiveConfig.Object, "spec", "logLevel")
			Expect(err).To(BeNil())
			Expect(found).To(BeTrue(), "couldn't find tolerations field in clustermanager")
			Expect(logLevel).To(BeEquivalentTo("debug"), "HiveConfig should have the logLevel spec set earlier: ", hiveConfig.Object)
		})

	}
}

var webhookTests = func() func() {
	return func() {
		It("blocks deletion if resouces exist", func() {
			key := &backplane.MultiClusterEngine{}
			Expect(k8sClient.Get(ctx, multiClusterEngine, key)).To(Succeed())
			ns := key.Spec.TargetNamespace

			resourcesDir := os.Getenv("RESOURCE_DIR")
			if resourcesDir == "" {
				resourcesDir = "../resources"
			}

			blockDeletionResources := []struct {
				Name     string
				GVK      schema.GroupVersionKind
				Filepath string
				crdPath  string
				Expected string
			}{
				{
					Name: "BareMetalAsset",
					GVK: schema.GroupVersionKind{
						Group:   "inventory.open-cluster-management.io",
						Version: "v1alpha1",
						Kind:    "BareMetalAsset",
					},
					Filepath: filepath.Join(resourcesDir, "baremetalassets.yaml"),
					Expected: "Existing BareMetalAsset resources must first be deleted",
				},
				{
					Name: "ManagedCluster",
					GVK: schema.GroupVersionKind{
						Group:   "cluster.open-cluster-management.io",
						Version: "v1",
						Kind:    "ManagedClusterList",
					},
					Filepath: filepath.Join(resourcesDir, "managedcluster.yaml"),
					Expected: "Existing ManagedCluster resources must first be deleted",
				},
			}
			for _, r := range blockDeletionResources {
				By("Creating a new "+r.Name, func() {

					if r.crdPath != "" {
						applyResource(r.crdPath, ns)
						defer deleteResource(r.crdPath, ns)
					}
					applyResource(r.Filepath, ns)
					defer deleteResource(r.Filepath, ns)

					config := &backplane.MultiClusterEngine{}
					Expect(k8sClient.Get(ctx, multiClusterEngine, config)).To(Succeed()) // Get multiClusterEngine

					err := k8sClient.Delete(ctx, config) // Attempt to delete multiClusterEngine. Ensure it does not succeed.
					Expect(err).ShouldNot(BeNil())
					Expect(err.Error()).Should(ContainSubstring(r.Expected))
				})
			}
		})

		It("Prevents modifications of the targetNamespace", func() {
			Eventually(func(g Gomega) {
				key := &backplane.MultiClusterEngine{}
				g.Expect(k8sClient.Get(ctx, multiClusterEngine, key)).To(Succeed())
				key.Spec.TargetNamespace = "shouldnotexist"
				err := k8sClient.Update(ctx, key)
				g.Expect(err).ShouldNot(BeNil())
				g.Expect(err.Error()).Should(ContainSubstring("changes cannot be made to target namespace"))
			}, 10*time.Second, time.Second).Should(Succeed())

		})

		It("Prevents illegal modifications of the availabilityConfig", func() {
			Eventually(func(g Gomega) {
				key := &backplane.MultiClusterEngine{}
				g.Expect(k8sClient.Get(ctx, multiClusterEngine, key)).To(Succeed())
				key.Spec.AvailabilityConfig = "shouldnotexist"
				err := k8sClient.Update(ctx, key)
				g.Expect(err).ShouldNot(BeNil())
				g.Expect(err.Error()).Should(ContainSubstring("Invalid AvailabilityConfig given"))
			}, 10*time.Second, time.Second).Should(Succeed())

		})

		It("Prevents disabling required components", func() {
			Eventually(func(g Gomega) {
				key := &backplane.MultiClusterEngine{}
				g.Expect(k8sClient.Get(ctx, multiClusterEngine, key)).To(Succeed())
				key.Disable("server-foundation")
				err := k8sClient.Update(ctx, key)
				g.Expect(err).ShouldNot(BeNil(), "webhook should not have allowed update")
				g.Expect(err.Error()).Should(ContainSubstring("invalid component config"))
			}, 10*time.Second, time.Second).Should(Succeed())
		})

		It("Prevents setting unknown components", func() {
			Eventually(func(g Gomega) {
				key := &backplane.MultiClusterEngine{}
				g.Expect(k8sClient.Get(ctx, multiClusterEngine, key)).To(Succeed())
				key.Spec.Components = append(key.Spec.Components, backplane.ComponentConfig{
					Name:    "unknown",
					Enabled: true,
				})
				err := k8sClient.Update(ctx, key)
				g.Expect(err).ShouldNot(BeNil(), "webhook should not have allowed update")
				g.Expect(err.Error()).Should(ContainSubstring("invalid component config"))
			}, 10*time.Second, time.Second).Should(Succeed())

		})
	}
}

func applyResource(resourceFile, namespace string) {
	resourceData, err := ioutil.ReadFile(resourceFile) // Get resource as bytes
	Expect(err).To(BeNil())

	unstructured := &unstructured.Unstructured{Object: map[string]interface{}{}}
	err = yaml.Unmarshal(resourceData, &unstructured.Object) // Render resource as unstructured
	Expect(err).To(BeNil())

	if unstructured.GetNamespace() != "" {
		unstructured.SetNamespace(namespace)
	}

	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Create(ctx, unstructured)).Should(Succeed()) // Create resource on cluster
	}, 30*time.Second, time.Second).Should(Succeed())

}

func deleteResource(resourceFile, namespace string) {
	resourceData, err := ioutil.ReadFile(resourceFile) // Get resource as bytes
	Expect(err).To(BeNil())

	unstructured := &unstructured.Unstructured{Object: map[string]interface{}{}}
	err = yaml.Unmarshal(resourceData, &unstructured.Object) // Render resource as unstructured
	Expect(err).To(BeNil())

	if unstructured.GetNamespace() != "" {
		unstructured.SetNamespace(namespace)
	}

	Expect(k8sClient.Delete(ctx, unstructured)).Should(Succeed()) // Delete resource on cluster
}

func defaultmultiClusterEngine() *backplane.MultiClusterEngine {
	return &backplane.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name: multiClusterEngineName,
		},
		Spec: backplane.MultiClusterEngineSpec{},
		Status: backplane.MultiClusterEngineStatus{
			Phase: "",
		},
	}
}

func validateDelete() {
	labelSelector := client.MatchingLabels{"multiClusterEngine.name": multiClusterEngine.Name}

	By("Checking for remaining services", func() {
		Eventually(func(g Gomega) {
			serviceList := &corev1.ServiceList{}
			g.Expect(k8sClient.List(ctx, serviceList, labelSelector)).To(Succeed())
			g.Expect(len(serviceList.Items)).To(BeZero())
		}, deleteTimeout, interval).Should(Succeed())
	})

	By("Checking for remaining serviceaccounts", func() {
		Eventually(func(g Gomega) {
			serviceAccountList := &corev1.ServiceAccountList{}
			g.Expect(k8sClient.List(ctx, serviceAccountList, labelSelector)).To(Succeed())
			g.Expect(len(serviceAccountList.Items)).To(BeZero())
		}, deleteTimeout, interval).Should(Succeed())
	})

	By("Checking for remaining deployments", func() {
		Eventually(func(g Gomega) {
			deploymentList := &appsv1.DeploymentList{}
			g.Expect(k8sClient.List(ctx, deploymentList, labelSelector)).To(Succeed())
			g.Expect(len(deploymentList.Items)).To(BeZero())
		}, deleteTimeout, interval).Should(Succeed())
	})

	By("Checking for remaining clusterroles", func() {
		Eventually(func(g Gomega) {
			clusterRoleList := &unstructured.UnstructuredList{}
			clusterRoleList.SetGroupVersionKind(
				schema.GroupVersionKind{
					Group:   "rbac.authorization.k8s.io",
					Version: "v1",
					Kind:    "ClusterRole",
				},
			)
			g.Expect(k8sClient.List(ctx, clusterRoleList, labelSelector)).To(Succeed())
			g.Expect(len(clusterRoleList.Items)).To(BeZero())
		}, deleteTimeout, interval).Should(Succeed())
	})

	By("Checking for remaining clusterrolebindings", func() {
		Eventually(func(g Gomega) {
			clusterRoleBindingList := &unstructured.UnstructuredList{}
			clusterRoleBindingList.SetGroupVersionKind(
				schema.GroupVersionKind{
					Group:   "rbac.authorization.k8s.io",
					Version: "v1",
					Kind:    "ClusterRoleBinding",
				},
			)
			g.Expect(k8sClient.List(ctx, clusterRoleBindingList, labelSelector)).To(Succeed())
			g.Expect(len(clusterRoleBindingList.Items)).To(BeZero())
		}, deleteTimeout, interval).Should(Succeed())
	})

	By("Checking for remaining apiservices", func() {
		Eventually(func(g Gomega) {
			apiServiceList := &unstructured.UnstructuredList{}
			apiServiceList.SetGroupVersionKind(
				schema.GroupVersionKind{
					Group:   "apiregistration.k8s.io",
					Version: "v1",
					Kind:    "APIService",
				},
			)
			g.Expect(k8sClient.List(ctx, apiServiceList, labelSelector)).To(Succeed())
			g.Expect(len(apiServiceList.Items)).To(BeZero())
		}, deleteTimeout, interval).Should(Succeed())
	})

	By("Checking for remaining clustermanager", func() {
		Eventually(func(g Gomega) {
			clusterManager := &unstructured.Unstructured{}
			clusterManager.SetGroupVersionKind(
				schema.GroupVersionKind{
					Group:   "operator.open-cluster-management.io",
					Version: "v1",
					Kind:    "ClusterManager",
				},
			)
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "cluster-manager"}, clusterManager)
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Expected IsNotFound error, got error:", err)
		}, deleteTimeout, interval).Should(Succeed())
	})

	By("Checking for remaining hiveconfig", func() {
		Eventually(func(g Gomega) {
			hiveConfig := &unstructured.Unstructured{}
			hiveConfig.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "hive.openshift.io",
				Version: "v1",
				Kind:    "HiveConfig",
			})
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "hive"}, hiveConfig)
			g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Expected IsNotFound error, got error:", err)
		}, deleteTimeout, interval).Should(Succeed())
	})

	By("Checking for remaining clusterManagementAddons", func() {
		Eventually(func(g Gomega) {
			cmaList := &unstructured.UnstructuredList{}
			cmaList.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "addon.open-cluster-management.io",
				Version: "v1alpha1",
				Kind:    "ClusterManagementAddOn",
			})
			err := k8sClient.List(ctx, cmaList, labelSelector)
			g.Expect(apierrors.IsUnexpectedServerError(err)).To(BeTrue(), "Expected CRD not to exist, got error:", err)
		}, deleteTimeout, interval).Should(Succeed())
	})
}

// validateResourceSpec scans clustermanager and deployments in namespace for the provided node selector, pull secret,
// and tolerations
func validateResourceSpecs(namespace string, nodeSelector map[string]string, pullSecret string, tolerations []corev1.Toleration, availabilityConfig backplane.AvailabilityType) {
	Eventually(func(g Gomega) {
		deployments := &appsv1.DeploymentList{}
		err := k8sClient.List(ctx, deployments,
			client.InNamespace(namespace),
			client.MatchingLabels{
				"backplaneconfig.name": multiClusterEngine.Name,
			})
		g.Expect(err).To(BeNil())
		g.Expect(len(deployments.Items)).ToNot(BeZero())
		availabilityList := []string{"managedcluster-import-controller-v2", "ocm-controller", "ocm-proxyserver", "ocm-webhook"}

		for _, deployment := range deployments.Items {
			// check nodeSelector
			componentSelector := deployment.Spec.Template.Spec.NodeSelector
			g.Expect(componentSelector).To(Equal(nodeSelector), fmt.Sprintf("Deployment %s does not have expected nodeselector", deployment.Name))

			// check imagePullSecret
			if pullSecret == "" {
				g.Expect(len(deployment.Spec.Template.Spec.ImagePullSecrets)).To(BeZero(), fmt.Sprintf("Deployment %s should not have imagepullsecrets", deployment.Name))
			} else {
				g.Expect(len(deployment.Spec.Template.Spec.ImagePullSecrets)).To(Not(BeZero()), fmt.Sprintf("Deployment %s does not have expected imagepullsecrets", deployment.Name))
				componentSecret := deployment.Spec.Template.Spec.ImagePullSecrets[0].Name
				g.Expect(componentSecret).To(Equal(pullSecret), fmt.Sprintf("Deployment %s does not have expected imagepullsecrets", deployment.Name))
			}

			// check tolerations
			componentTolerations := deployment.Spec.Template.Spec.Tolerations
			g.Expect(componentTolerations).To(Equal(tolerations), fmt.Sprintf("Deployment %s does not have expected tolerations", deployment.Name))

			// check replicas
			if contains(availabilityList, deployment.ObjectMeta.Name) {
				componentReplicas := deployment.Spec.Replicas
				if (availabilityConfig == backplane.HAHigh) || (availabilityConfig == "") {
					g.Expect(*componentReplicas).To(Equal(int32(2)), fmt.Sprintf("Deployment %s does not have expected replicas", deployment.Name))
				}
				if availabilityConfig == backplane.HABasic {
					g.Expect(*componentReplicas).To(Equal(int32(1)), fmt.Sprintf("Deployment %s does not have expected replicas", deployment.Name))
				}
			}

		}

		// check clustermanager
		clusterManager := &unstructured.Unstructured{}
		clusterManager.SetGroupVersionKind(
			schema.GroupVersionKind{
				Group:   "operator.open-cluster-management.io",
				Version: "v1",
				Kind:    "ClusterManager",
			},
		)
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "cluster-manager"}, clusterManager)
		g.Expect(err).To(BeNil())
		componentTolerations, found, err := unstructured.NestedSlice(clusterManager.Object, "spec", "nodePlacement", "tolerations")
		g.Expect(err).To(BeNil())
		g.Expect(found).To(BeTrue(), "couldn't find tolerations field in clustermanager")
		// TODO: find a better way to compare tolerations
		g.Expect(len(componentTolerations)).To(BeNumerically("==", len(tolerations)), fmt.Sprintf("%v did not match %v", componentTolerations, tolerations))

		componentSelector, found, err := unstructured.NestedMap(clusterManager.Object, "spec", "nodePlacement", "nodeSelector")
		g.Expect(err).To(BeNil())
		if nodeSelector == nil {
			g.Expect(found).To(BeFalse(), "nodeselector field in clustermanager")
		} else {
			g.Expect(found).To(BeTrue(), "couldn't find nodeselector field in clustermanager")
			g.Expect(len(componentSelector)).To(BeNumerically("==", len(nodeSelector)))
		}

	}, time.Minute, time.Second).Should(Succeed())
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}
