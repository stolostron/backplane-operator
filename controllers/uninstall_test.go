// Copyright Contributors to the Open Cluster Management project

/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	configv1 "github.com/openshift/api/config/v1"
	v1 "github.com/stolostron/backplane-operator/api/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	//+kubebuilder:scaffold:imports
)

// Define utility constants for object names and testing timeouts/durations and intervals.

var _ = Describe("BackplaneConfig controller", func() {
	var (
		tests testList
	)

	AfterEach(func() {
		Expect(k8sClient.Delete(context.Background(), &configv1.ClusterVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name: "version",
			},
		})).To(Succeed())
		Expect(k8sClient.Delete(context.Background(), &configv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster",
			},
		})).To(Succeed())
		Expect(k8sClient.Delete(context.Background(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testsecret",
				Namespace: DestinationNamespace,
			},
		})).To(Succeed())
	})

	BeforeEach(func() {
		// Create openshift-monitoring namespace because metrics stands up prometheus endpoint here
		err := k8sClient.Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "openshift-monitoring",
			},
			Spec: corev1.NamespaceSpec{},
		})
		if err != nil && !errors.IsAlreadyExists(err) {
			Expect(err).To(BeNil())
		}

		// Create target namespace
		err = k8sClient.Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: DestinationNamespace,
			},
			Spec: corev1.NamespaceSpec{},
		})
		if err != nil && !errors.IsAlreadyExists(err) {
			Expect(err).To(BeNil())
		}
		// Create ClusterVersion
		// Attempted to Store Version in status. Unable to get it to stick.
		Expect(k8sClient.Create(context.Background(), &configv1.ClusterVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name: "version",
			},
			Spec: configv1.ClusterVersionSpec{
				Channel:   "stable-4.9",
				ClusterID: "12345678910",
			},
		})).To(Succeed())

		// Create ClusterIngress
		// Attempted to Store Version in status. Unable to get it to stick.
		Expect(k8sClient.Create(context.Background(), &configv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name: "cluster",
			},
			Spec: configv1.IngressSpec{
				Domain: "apps.installer-test-cluster.dev00.red-chesterfield.com",
			},
		})).To(Succeed())

		Expect(k8sClient.Create(context.Background(), &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "hypershift-deployment",
				Namespace: DestinationNamespace,
			},
		})).To(Succeed())

		// Create test secret in target namespace
		testsecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testsecret",
				Namespace: DestinationNamespace,
			},
		}
		Eventually(func() error {
			err := k8sClient.Create(context.TODO(), testsecret)
			if apierrors.IsAlreadyExists(err) {
				return nil
			}
			return err
		}, timeout, interval).Should(Succeed())

	})

	When("creating a new BackplaneConfig", func() {

		Context("and no image pull policy is specified", func() {
			It("should remove old components", func() {
				// createCtx := context.Background()
				By("creating the backplane config")
				backplaneConfig := &v1.MultiClusterEngine{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "multicluster.openshift.io/v1",
						Kind:       "MultiClusterEngine",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: BackplaneConfigName,
					},
					Spec: v1.MultiClusterEngineSpec{
						TargetNamespace: DestinationNamespace,
						ImagePullSecret: "testsecret",
					},
				}

				reconciler.ensureRemovalsGone(backplaneConfig)
				for _, test := range tests {
					By(fmt.Sprintf("ensuring %s is creatremoved", test.Name))
					Eventually(func() bool {
						ctx := context.Background()
						err := k8sClient.Get(ctx, test.NamespacedName, test.ResourceType)
						return errors.IsNotFound(err)
					}, timeout, interval).Should(BeTrue())
				}

			})
		})

	})
})
