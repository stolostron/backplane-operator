// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package backplane_install_test

import (
	"context"
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
	existingNamespace      = "existing-ns"
	newNamespace           = "new-ns"
	defaultTrustBundleName = "trusted-ca-bundle"
	trustBundleFileName    = "ca-bundle.crt"

	consoleVersionConstraint = ">= 4.10.0-0"

	// installTimeout is the max time an install should take
	installTimeout = time.Minute * 5

	// deleteTimeout is the max time a delete should take
	deleteTimeout = time.Minute * 3

	// validateTimeout is the max time the validation should take
	validateTimeout = time.Minute

	// trustTimeout is the max time a trust bundle ConfigMap operation should take
	trustTimeout = time.Minute

	duration = time.Second * 15
	interval = time.Second
)

var (
	ctx = context.Background()

	k8sClient client.Client

	multiClusterEngine = types.NamespacedName{
		Name: multiClusterEngineName,
	}
)

var _ = Describe("MultiClusterEngine", func() {
	Context("when using an empty MCE spec", func() {
		mce := defaultMultiClusterEngine()
		fullTestSuite(mce)
	})

	Context("when targeting an existing namespace", func() {
		mce := defaultMultiClusterEngine()
		mce.Spec.TargetNamespace = existingNamespace

		It("should create a namespace to install to", func() {
			err := k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: existingNamespace},
			})
			if err != nil {
				Expect(apierrors.IsAlreadyExists(err)).To(BeTrue())
			}
		})

		fullTestSuite(mce)

		It("should preserve the existing namespace", func() {
			createdNS := &corev1.Namespace{}
			By("ensuring the existing namespace remains")
			Consistently(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: existingNamespace}, createdNS)).To(Succeed())
			}, deleteTimeout, interval).Should(Succeed())

			By("cleaning up the created namespace")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Delete(ctx, createdNS)).Should(Succeed())
			}, deleteTimeout, interval).Should(Succeed())
		})
	})

	Context("when targeting a new namespace", func() {
		mce := defaultMultiClusterEngine()
		mce.Spec.TargetNamespace = newNamespace

		fullTestSuite(mce)

		It("should remove the new namespace", func() {
			Eventually(func(g Gomega) {
				createdNS := &corev1.Namespace{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: newNamespace}, createdNS)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Expected IsNotFound error, got error:", err)
			}, deleteTimeout, interval).Should(Succeed())
		})
	})

})

// complete set of MCE tests
var fullTestSuite = func(mce *backplane.MultiClusterEngine) {
	It("should install all components", func() {
		By("by creating a new BackplaneConfig", func() {
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Create(ctx, mce)).Should(Succeed())
			}, duration, interval).Should(Succeed())
		})
	})

	Context("when performing an install", installTests())
	Context("when checking webhooks", webhookTests())
	Context("when self healing the operator", selfHealingTests())
	Context("when checking configuration options", configurationTests())

	It("should remove mce", func() {
		By("deleting BackplaneConfig", func() {
			Eventually(func(g Gomega) {
				err := k8sClient.Delete(ctx, mce)
				if err != nil {
					Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Error calling delete on MCE", err.Error())
				}
			}, deleteTimeout, interval).Should(Succeed())
		})

		By("ensuring the BackplaneConfig was deleted", func() {
			Eventually(func() bool {
				err := k8sClient.Get(ctx, multiClusterEngine, &backplane.MultiClusterEngine{})
				if err == nil {
					return false
				}
				return apierrors.IsNotFound(err)
			}, deleteTimeout, interval).Should(BeTrue(), "The backplane config was still found after deletion")
		})
	})

	Context("when uninstalling the operator", uninstallTests())
}

var validateMCEConsoleTests = func(mce *backplane.MultiClusterEngine) {
	By("getting current OCP version")
	clusterVersion := &configv1.ClusterVersion{}
	clusterVersionKey := types.NamespacedName{Name: "version"}
	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, clusterVersionKey, clusterVersion)).To(Succeed())
		state := clusterVersion.Status.History[0].State
		g.Expect(state).Should(
			Equal(configv1.CompletedUpdate),
			"Expected CompletedUpdate status in clusterVersion resource",
		)
	}, duration, interval).Should(Succeed())
	version := clusterVersion.Status.History[0].Version
	currentVersion, err := semver.NewVersion(version)
	Expect(err).To(BeNil(), "Error creating current OCP version")

	By("creating a version constraint for when the console should be installed")
	mceConsoleConstraint, err := semver.NewConstraint(consoleVersionConstraint)
	Expect(err).To(BeNil(), "Error creating console version constraint")

	By("checking the console version constraint")
	if mceConsoleConstraint.Check(currentVersion) {
		By("ensuring the MCE console is installed when the OCP version is >= 4.10")
		components := mce.Status.Components
		found := false
		for _, component := range components {
			if component.Name == "console-mce-console" {
				found = true
				break
			}
		}
		Expect(found).To(BeTrue(), "Expected MCE Console to be found in MultiClusterEngine status")

		By("ensuring mce plugin is enabled in openshift console")
		console := &operatorv1.Console{}
		consoleKey := types.NamespacedName{Name: "cluster"}
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, consoleKey, console)).To(Succeed())
		}, duration, interval).Should(Succeed())
		pluginsList := console.Spec.Plugins
		Expect(contains(pluginsList, "mce")).To(BeTrue(), "Expected MCE plugin to be enabled in console resource")
	} else {
		By("ensuring the MCE console is not installed when the OCP version is < 4.10")
		components := mce.Status.Components
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
			Eventually(func(g Gomega) {
				mce := &backplane.MultiClusterEngine{}
				g.Expect(k8sClient.Get(ctx, multiClusterEngine, mce)).To(Succeed())
				available := mce.Status.Phase == backplane.MultiClusterEnginePhaseAvailable
				hasMinCount := len(mce.Status.Components) > 5
				g.Expect(available && hasMinCount).To(BeTrue())
			}, installTimeout, interval).Should(Succeed())

		})

		It("should have a healthy status", func() {
			mce := &backplane.MultiClusterEngine{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, multiClusterEngine, mce)).To(Succeed())

				By("checking the phase", func() {
					g.Expect(mce.Status.Phase).To(Equal(backplane.MultiClusterEnginePhaseAvailable))
				})
				By("checking the components", func() {
					g.Expect(len(mce.Status.Components)).Should(BeNumerically(">=", 6), "Expected at least 6 components in status")
				})
				By("validating the MCE Console", func() {
					validateMCEConsoleTests(mce)
				})
				By("checking the conditions", func() {
					available := backplane.MultiClusterEngineCondition{}
					for _, c := range mce.Status.Conditions {
						if c.Type == backplane.MultiClusterEngineAvailable {
							available = c
						}
					}
					g.Expect(available.Status).To(Equal(metav1.ConditionTrue))
				})
			}, duration, interval).Should(Succeed())
		})

		It("should ensure the trusted CA bundle ConfigMap is created and populated", func() {
			mce := &backplane.MultiClusterEngine{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, multiClusterEngine, mce)).To(Succeed())
			}, duration, interval).Should(Succeed())
			nn := types.NamespacedName{
				Name:      defaultTrustBundleName,
				Namespace: mce.Spec.TargetNamespace,
			}
			cm := &corev1.ConfigMap{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, multiClusterEngine, mce)).To(Succeed())
				g.Expect(k8sClient.Get(ctx, nn, cm)).To(Succeed())
				data, ok := cm.Data[trustBundleFileName]
				g.Expect(ok).To(BeTrue(), "%s file should be in trust bundle ConfigMap", trustBundleFileName)
				g.Expect(len(data) > 0).To(BeTrue(), "%s file was empty", trustBundleFileName)
			}, trustTimeout, interval).Should(Succeed())
		})
	}
}

var uninstallTests = func() func() {
	return func() {
		It("should ensure all components have been cleaned up", func() {
			labelSelector := client.MatchingLabels{"multiClusterEngine.name": multiClusterEngine.Name}

			By("checking for remaining services", func() {
				Eventually(func(g Gomega) {
					serviceList := &corev1.ServiceList{}
					g.Expect(k8sClient.List(ctx, serviceList, labelSelector)).To(Succeed())
					g.Expect(len(serviceList.Items)).To(BeZero())
				}, duration, interval).Should(Succeed())
			})

			By("checking for remaining serviceaccounts", func() {
				Eventually(func(g Gomega) {
					serviceAccountList := &corev1.ServiceAccountList{}
					g.Expect(k8sClient.List(ctx, serviceAccountList, labelSelector)).To(Succeed())
					g.Expect(len(serviceAccountList.Items)).To(BeZero())
				}, duration, interval).Should(Succeed())
			})

			By("checking for remaining deployments", func() {
				Eventually(func(g Gomega) {
					deploymentList := &appsv1.DeploymentList{}
					g.Expect(k8sClient.List(ctx, deploymentList, labelSelector)).To(Succeed())
					g.Expect(len(deploymentList.Items)).To(BeZero())
				}, duration, interval).Should(Succeed())
			})

			By("checking for remaining clusterroles", func() {
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
				}, duration, interval).Should(Succeed())
			})

			By("checking for remaining clusterrolebindings", func() {
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
				}, duration, interval).Should(Succeed())
			})

			By("checking for remaining apiservices", func() {
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
				}, duration, interval).Should(Succeed())
			})

			By("checking for remaining clustermanager", func() {
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
				}, duration, interval).Should(Succeed())
			})

			By("checking for remaining hiveconfig", func() {
				Eventually(func(g Gomega) {
					hiveConfig := &unstructured.Unstructured{}
					hiveConfig.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "hive.openshift.io",
						Version: "v1",
						Kind:    "HiveConfig",
					})
					err := k8sClient.Get(ctx, types.NamespacedName{Name: "hive"}, hiveConfig)
					g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Expected IsNotFound error, got error:", err)
				}, duration, interval).Should(Succeed())
			})

			By("checking for remaining clusterManagementAddons", func() {
				Eventually(func(g Gomega) {
					cmaList := &unstructured.UnstructuredList{}
					cmaList.SetGroupVersionKind(schema.GroupVersionKind{
						Group:   "addon.open-cluster-management.io",
						Version: "v1alpha1",
						Kind:    "ClusterManagementAddOn",
					})
					err := k8sClient.List(ctx, cmaList, labelSelector)
					g.Expect(apierrors.IsUnexpectedServerError(err)).To(BeTrue(), "Expected CRD not to exist, got error:", err)
				}, duration, interval).Should(Succeed())
			})
		})
	}
}

// configurationTests test variations in mce spec options
var configurationTests = func() func() {
	return func() {
		Context("when using the default backplane operator config", func() {
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
				mce := &backplane.MultiClusterEngine{}
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, multiClusterEngine, mce)).To(Succeed())
				}, duration, interval).Should(Succeed())

				validateResourceSpecs(mce.Spec.TargetNamespace, defaultNodeSelector, defaultPullSecret, defaultTolerations, defaultAvailabilityConfig)
			})

		})

		Context("customized imagePullSecret", func() {
			backplanePullSecret := "test"

			It("should error due to missing secret", func() {
				key := &backplane.MultiClusterEngine{}
				Expect(k8sClient.Get(ctx, multiClusterEngine, key)).To(Succeed())

				// Delete secret if it already exists
				secretNN := types.NamespacedName{Name: backplanePullSecret, Namespace: key.Spec.TargetNamespace}
				targetSecret := &corev1.Secret{}
				err := k8sClient.Get(ctx, secretNN, targetSecret)
				if err == nil {
					Expect(k8sClient.Delete(ctx, targetSecret)).To(Succeed())
				}

				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, multiClusterEngine, key)).To(Succeed())
					key.Spec.ImagePullSecret = backplanePullSecret
					g.Expect(k8sClient.Update(ctx, key)).To(Succeed())
				}, 10*time.Second, time.Second).Should(Succeed())

				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, multiClusterEngine, key)).To(Succeed())
					g.Expect(key.Status.Phase).To(Equal(backplane.MultiClusterEnginePhaseError))
				}, 15*time.Second, interval).Should(Succeed())
			})

			It("should resolve error by adding missing secret", func() {
				key := &backplane.MultiClusterEngine{}
				Expect(k8sClient.Get(ctx, multiClusterEngine, key)).To(Succeed())
				ns := key.Spec.TargetNamespace

				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      backplanePullSecret,
						Namespace: ns,
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, multiClusterEngine, key)).To(Succeed())
					g.Expect(key.Status.Phase).To(Equal(backplane.MultiClusterEnginePhaseProgressing))
				}, 30*time.Second, interval).Should(Succeed())
			})
		})

		Context("when using a customized backplane operator config", func() {
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
				mce := &backplane.MultiClusterEngine{}

				// retry update due to conflicts
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, multiClusterEngine, mce)).To(Succeed())
					mce.Spec.NodeSelector = backplaneNodeSelector
					mce.Spec.ImagePullSecret = backplanePullSecret
					mce.Spec.Tolerations = backplaneTolerations
					mce.Spec.AvailabilityConfig = backplaneAvailabilityConfig
					g.Expect(k8sClient.Update(ctx, mce)).To(Succeed())
				}, duration, interval).Should(Succeed())

				validateResourceSpecs(mce.Spec.TargetNamespace, backplaneNodeSelector, backplanePullSecret, backplaneTolerations, backplaneAvailabilityConfig)
			})
		})

		Context("when toggling components in the backplane operator config", func() {
			It("should disable discovery and enable hypershift", func() {
				mce := &backplane.MultiClusterEngine{}

				By("modifying the toggles in backplane operator config", func() {
					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, multiClusterEngine, mce)).To(Succeed())
						mce.Disable(backplane.Discovery)
						mce.Enable(backplane.HyperShift)
						g.Expect(k8sClient.Update(ctx, mce)).To(Succeed())
					}, duration, interval).Should(Succeed())
				})

				namespace := mce.Spec.TargetNamespace
				deploy := &appsv1.Deployment{}

				By("ensuring discover operator is disabled", func() {
					discoveryNN := types.NamespacedName{
						Name:      "discovery-operator",
						Namespace: namespace,
					}
					Eventually(func(g Gomega) {
						err := k8sClient.Get(ctx, discoveryNN, deploy)
						g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "Expected IsNotFound error, got error:", err)
					}, duration, interval).Should(Succeed())

					By("ensuring hypershift is enabled", func() {
						hyperAddonNN := types.NamespacedName{
							Name:      "hypershift-addon-manager",
							Namespace: namespace,
						}
						Eventually(func(g Gomega) {
							err := k8sClient.Get(ctx, hyperAddonNN, deploy)
							g.Expect(err).To(BeNil(), "Expected no error, got error:", err)
						}, duration, interval).Should(Succeed())
					})
				})
			})
		})

	}
}

// selfHealingTests test that changes to components are auto-corrected
var selfHealingTests = func() func() {
	return func() {
		It("be able to modify the hive config", func() {
			hiveConfig := &unstructured.Unstructured{}
			hiveConfig.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "hive.openshift.io",
				Version: "v1",
				Kind:    "HiveConfig",
			})
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "hive"}, hiveConfig)).To(Succeed())
			}, duration, interval).Should(Succeed())
			unstructured.SetNestedField(hiveConfig.Object, "debug", "spec", "logLevel")
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Update(ctx, hiveConfig)).Should(Succeed())
			}, duration, interval).Should(Succeed())
		})

		It("should ensure the backplane is self-correcting", func() {
			mce := &backplane.MultiClusterEngine{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, multiClusterEngine, mce)).To(Succeed())
			}, duration, interval).Should(Succeed())
			namespace := mce.Spec.TargetNamespace

			By("checking metadata is maintained but not overwritten", func() {
				By("manipulating annotations and backplane labels in the ocm-controller deployment", func() {
					deploy := &appsv1.Deployment{}
					nn := types.NamespacedName{Name: "ocm-controller", Namespace: namespace}

					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, nn, deploy)).To(Succeed())
					}, duration, interval).Should(Succeed())

					deploy.SetLabels(map[string]string{})
					deploy.SetAnnotations(map[string]string{"testannotation": "test"})

					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Update(ctx, deploy)).Should(Succeed())
					}, duration, interval).Should(Succeed())
				})

				By("checking backplane labels are added back and custom annotations are preserved", func() {
					deploy := &appsv1.Deployment{}
					nn := types.NamespacedName{Name: "ocm-controller", Namespace: namespace}
					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, nn, deploy)).To(Succeed())
						l := deploy.GetLabels()
						g.Expect(l["backplaneconfig.name"]).To(Equal(multiClusterEngine.Name), "Missing backplane label")
						a := deploy.GetAnnotations()
						g.Expect(a["testannotation"]).To(Equal("test"), "Test annotation may have been stripped out of deployment")
					}, duration, interval).Should(Succeed())
				})
			})

			By("checking spec changes are set to their expected values", func() {
				By("manipulating the spec in the ocm-controller deployment", func() {
					targetDeploy := &appsv1.Deployment{}
					nn := types.NamespacedName{Name: "ocm-controller", Namespace: namespace}
					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, nn, targetDeploy)).To(Succeed())
					}, duration, interval).Should(Succeed())

					targetDeploy.Spec.Template.Spec.ServiceAccountName = "test-sa"
					targetDeploy.SetAnnotations(map[string]string{"testannotation": "test2"})
					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Update(ctx, targetDeploy)).Should(Succeed())
					}, duration, interval).Should(Succeed())
				})

				By("confirming the spec is reset", func() {
					Eventually(func(g Gomega) {
						deploy := &appsv1.Deployment{}
						g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ocm-controller", Namespace: mce.Spec.TargetNamespace}, deploy)).To(Succeed())

						// annotation is used to verify the deployment has been updated
						a := deploy.GetAnnotations()
						g.Expect(a["testannotation"]).To(Equal("test2"), "Deployment may not have been updated")

						g.Expect(deploy.Spec.Template.Spec.ServiceAccountName).NotTo(Equal("test-sa"), "Deployment restart policy change was not reverted by operator")
					}, duration, interval).Should(Succeed())
				})
			})

			By("recreating the trusted bundle ConfigMap if deleted", func() {
				nn := types.NamespacedName{
					Name:      defaultTrustBundleName,
					Namespace: namespace,
				}
				cm := &corev1.ConfigMap{}

				By("deleting the trusted CA bundle ConfigMap", func() {
					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, nn, cm)).To(Succeed())
					}, duration, interval).Should(Succeed())

					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Delete(ctx, cm)).Should(Succeed())
					}, deleteTimeout, interval).Should(Succeed())
				})

				By("ensuring the trusted CA bundle ConfigMap is recreated", func() {
					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, nn, cm)).To(Succeed())
						data, ok := cm.Data[trustBundleFileName]
						g.Expect(ok).To(BeTrue(), "%s file should be in trust bundle ConfigMap", trustBundleFileName)
						g.Expect(len(data) > 0).To(BeTrue(), "%s file was empty", trustBundleFileName)
					}, deleteTimeout, interval).Should(Succeed())
				})
			})
		})

		It("should ensure the Hiveconfig modifications are preserved", func() {
			hiveConfig := &unstructured.Unstructured{}
			hiveConfig.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "hive.openshift.io",
				Version: "v1",
				Kind:    "HiveConfig",
			})
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "hive"}, hiveConfig)).To(Succeed())
			}, duration, interval).Should(Succeed())
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
			mce := &backplane.MultiClusterEngine{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, multiClusterEngine, mce)).To(Succeed())
			}, duration, interval).Should(Succeed())
			ns := mce.Spec.TargetNamespace

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
				By("creating a new "+r.Name, func() {

					if r.crdPath != "" {
						applyResource(r.crdPath, ns)
						defer deleteResource(r.crdPath, ns)
					}
					applyResource(r.Filepath, ns)
					defer deleteResource(r.Filepath, ns)

					config := &backplane.MultiClusterEngine{}
					Eventually(func(g Gomega) {
						g.Expect(k8sClient.Get(ctx, multiClusterEngine, config)).To(Succeed()) // Get multiClusterEngine
					}, duration, interval).Should(Succeed())

					Eventually(func(g Gomega) {
						err := k8sClient.Delete(ctx, config) // Attempt to delete multiClusterEngine. Ensure it does not succeed.
						g.Expect(err).ShouldNot(BeNil())
						g.Expect(err.Error()).Should(ContainSubstring(r.Expected))
					}, deleteTimeout, interval).Should(Succeed())
				})
			}
		})

		It("blocks creation if targetNamespace in use", func() {
			mce := &backplane.MultiClusterEngine{}
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, multiClusterEngine, mce)).To(Succeed())
			}, duration, interval).Should(Succeed())

			newMCE := defaultMultiClusterEngine()
			newMCE.Name = "newName"
			newMCE.Spec.TargetNamespace = mce.Spec.TargetNamespace
			Expect(k8sClient.Create(ctx, mce)).ToNot(BeNil())
		})

		It("prevents modifications of the targetNamespace", func() {
			Eventually(func(g Gomega) {
				mce := &backplane.MultiClusterEngine{}
				g.Expect(k8sClient.Get(ctx, multiClusterEngine, mce)).To(Succeed())
				mce.Spec.TargetNamespace = "shouldnotexist"
				err := k8sClient.Update(ctx, mce)
				g.Expect(err).ShouldNot(BeNil())
				g.Expect(err.Error()).Should(ContainSubstring("changes cannot be made to target namespace"))
			}, duration, interval).Should(Succeed())

		})

		It("prevents modifications of the deploymentMode", func() {
			Eventually(func(g Gomega) {
				mce := &backplane.MultiClusterEngine{}
				g.Expect(k8sClient.Get(ctx, multiClusterEngine, mce)).To(Succeed())
				if mce.Spec.DeploymentMode == backplane.ModeHosted {
					mce.Spec.DeploymentMode = backplane.ModeStandalone
				} else {
					mce.Spec.DeploymentMode = backplane.ModeHosted
				}
				err := k8sClient.Update(ctx, mce)
				g.Expect(err).ShouldNot(BeNil())
				g.Expect(err.Error()).Should(ContainSubstring("changes cannot be made to DeploymentMode"))
			}, duration, interval).Should(Succeed())

		})

		It("prevents illegal modifications of the availabilityConfig", func() {
			Eventually(func(g Gomega) {
				mce := &backplane.MultiClusterEngine{}
				g.Expect(k8sClient.Get(ctx, multiClusterEngine, mce)).To(Succeed())
				mce.Spec.AvailabilityConfig = "shouldnotexist"
				err := k8sClient.Update(ctx, mce)
				g.Expect(err).ShouldNot(BeNil())
				g.Expect(err.Error()).Should(ContainSubstring("Invalid AvailabilityConfig given"))
			}, duration, interval).Should(Succeed())

		})

		It("prevents setting unknown components", func() {
			Eventually(func(g Gomega) {
				mce := &backplane.MultiClusterEngine{}
				g.Expect(k8sClient.Get(ctx, multiClusterEngine, mce)).To(Succeed())
				mce.Spec.Overrides.Components = append(mce.Spec.Overrides.Components, backplane.ComponentConfig{
					Name:    "unknown",
					Enabled: true,
				})
				err := k8sClient.Update(ctx, mce)
				g.Expect(err).ShouldNot(BeNil(), "webhook should not have allowed update")
				g.Expect(err.Error()).Should(ContainSubstring("invalid component config"))
			}, duration, interval).Should(Succeed())

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
	}, duration, interval).Should(Succeed())

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

	Eventually(func(g Gomega) {
		Expect(k8sClient.Delete(ctx, unstructured)).Should(Succeed()) // Delete resource on cluster
	}, deleteTimeout, interval).Should(Succeed())
}

func defaultMultiClusterEngine() *backplane.MultiClusterEngine {
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

// validateResourceSpec scans clustermanager and deployments in namespace for the provided node selector, pull secret,
// and tolerations
func validateResourceSpecs(namespace string, nodeSelector map[string]string, pullSecret string, tolerations []corev1.Toleration, availabilityConfig backplane.AvailabilityType) {
	Eventually(func(g Gomega) {
		By("listing deployments")
		deployments := &appsv1.DeploymentList{}
		err := k8sClient.List(ctx, deployments,
			client.InNamespace(namespace),
			client.MatchingLabels{
				"backplaneconfig.name": multiClusterEngine.Name,
			},
		)
		g.Expect(err).To(BeNil())
		g.Expect(len(deployments.Items)).ToNot(BeZero())
		availabilityList := []string{"managedcluster-import-controller-v2", "ocm-controller", "ocm-proxyserver", "ocm-webhook"}

		for _, deployment := range deployments.Items {
			By("checking the nodeSelector")
			componentSelector := deployment.Spec.Template.Spec.NodeSelector
			g.Expect(componentSelector).To(Equal(nodeSelector), "Deployment %s does not have expected nodeselector", deployment.Name)

			By("checking the imagePullSecret")
			if pullSecret == "" {
				g.Expect(len(deployment.Spec.Template.Spec.ImagePullSecrets)).To(
					BeZero(),
					"Deployment %s should not have imagepullsecrets", deployment.Name,
				)
			} else {
				g.Expect(len(deployment.Spec.Template.Spec.ImagePullSecrets)).To(
					Not(BeZero()),
					"Deployment %s does not have expected imagepullsecrets", deployment.Name,
				)
				componentSecret := deployment.Spec.Template.Spec.ImagePullSecrets[0].Name
				g.Expect(componentSecret).To(
					Equal(pullSecret),
					"Deployment %s does not have expected imagepullsecrets", deployment.Name,
				)
			}

			By("checking tolerations")
			componentTolerations := deployment.Spec.Template.Spec.Tolerations
			g.Expect(componentTolerations).To(
				Equal(tolerations),
				"Deployment %s does not have expected tolerations", deployment.Name,
			)

			By("checking replicas")
			if contains(availabilityList, deployment.ObjectMeta.Name) {
				componentReplicas := *deployment.Spec.Replicas
				if (availabilityConfig == backplane.HAHigh) || (availabilityConfig == "") {
					g.Expect(componentReplicas).To(
						Equal(int32(2)),
						"Deployment %s does not have expected replicas", deployment.Name,
					)
				}
				if availabilityConfig == backplane.HABasic {
					g.Expect(componentReplicas).To(
						Equal(int32(1)),
						"Deployment %s does not have expected replicas", deployment.Name,
					)
				}
			}

		}

		By("getting the clustermanager")
		clusterManager := &unstructured.Unstructured{}
		clusterManager.SetGroupVersionKind(
			schema.GroupVersionKind{
				Group:   "operator.open-cluster-management.io",
				Version: "v1",
				Kind:    "ClusterManager",
			},
		)
		g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cluster-manager"}, clusterManager)).To(Succeed())

		By("checking the clustermanager's tolerations")
		componentTolerations, found, err := unstructured.NestedSlice(clusterManager.Object, "spec", "nodePlacement", "tolerations")
		g.Expect(err).To(BeNil())
		g.Expect(found).To(BeTrue(), "couldn't find tolerations field in clustermanager")

		g.Expect(len(componentTolerations)).To(
			Equal(len(tolerations)),
			"tolerations %v did not match expected %v", componentTolerations, tolerations,
		)

		By("checking the clustermanager's node selectors")
		componentSelector, found, err := unstructured.NestedMap(clusterManager.Object, "spec", "nodePlacement", "nodeSelector")
		g.Expect(err).To(BeNil())
		if nodeSelector == nil {
			g.Expect(found).To(BeFalse(), "unexpected nodeselector field in clustermanager")
		} else {
			g.Expect(found).To(BeTrue(), "nodeselector field not found in clustermanager")
			g.Expect(len(componentSelector)).To(
				Equal(len(nodeSelector)),
				"node selectors %v did not match expected %v",
				componentSelector, nodeSelector,
			)
		}
	}, validateTimeout, interval).Should(Succeed())
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}
