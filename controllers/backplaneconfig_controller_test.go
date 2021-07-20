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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/open-cluster-management/backplane-operator/api/v1alpha1"
)

var _ = Describe("BackplaneConfig controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		BackplaneConfigName      = "test-backplaneconfig"
		BackplaneConfigNamespace = "default"
		JobName                  = "test-job"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When creating BackplaneConfig", func() {
		It("Should create cluster-manager custom resource", func() {
			By("By creating a new BackplaneConfig")
			ctx := context.Background()
			backplaneConfig := &v1alpha1.BackplaneConfig{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "backplane.open-cluster-management.io/v1alpha1",
					Kind:       "BackplaneConfig",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      BackplaneConfigName,
					Namespace: BackplaneConfigNamespace,
				},
				Spec: v1alpha1.BackplaneConfigSpec{},
			}
			Expect(k8sClient.Create(ctx, backplaneConfig)).Should(Succeed())

			backplaneConfigLookupKey := types.NamespacedName{Name: BackplaneConfigName, Namespace: BackplaneConfigNamespace}
			createdBackplaneConfig := &v1alpha1.BackplaneConfig{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, backplaneConfigLookupKey, createdBackplaneConfig)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			clusterManagerLookupKey := types.NamespacedName{Name: "cluster-manager"}
			clusterManager := &unstructured.Unstructured{}
			clusterManager.SetGroupVersionKind(schema.GroupVersionKind{Group: "operator.open-cluster-management.io", Version: "v1", Kind: "ClusterManager"})
			Eventually(func() bool {
				err := k8sClient.Get(ctx, clusterManagerLookupKey, clusterManager)
				return err == nil
			}, timeout, interval).Should(BeTrue())

			hiveConfigLookupKey := types.NamespacedName{Name: "hive"}
			hiveConfig := &unstructured.Unstructured{}
			hiveConfig.SetGroupVersionKind(schema.GroupVersionKind{Group: "hive.openshift.io", Version: "v1", Kind: "HiveConfig"})
			Eventually(func() bool {
				err := k8sClient.Get(ctx, hiveConfigLookupKey, hiveConfig)
				return err == nil
			}, timeout, interval).Should(BeTrue())
		})
	})
})
