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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	client "sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

var _ = Describe("BackplaneConfig controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		BackplaneConfigName        = "test-backplaneconfig"
		BackplaneOperatorNamespace = "default"
		JobName                    = "test-job"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	clusterManager := &unstructured.Unstructured{}
	clusterManager.SetGroupVersionKind(schema.GroupVersionKind{Group: "operator.open-cluster-management.io", Version: "v1", Kind: "ClusterManager"})
	hiveConfig := &unstructured.Unstructured{}
	hiveConfig.SetGroupVersionKind(schema.GroupVersionKind{Group: "hive.openshift.io", Version: "v1", Kind: "HiveConfig"})
	tests := []struct {
		Name           string
		NamespacedName types.NamespacedName
		ResourceType   client.Object
		Expected       error
	}{
		{
			Name:           "Backplane Config",
			NamespacedName: types.NamespacedName{Name: BackplaneConfigName},
			ResourceType:   &v1alpha1.BackplaneConfig{},
			Expected:       nil,
		},
		{
			Name:           "OCM Webhook",
			NamespacedName: types.NamespacedName{Name: "ocm-webhook", Namespace: BackplaneOperatorNamespace},
			ResourceType:   &appsv1.Deployment{},
			Expected:       nil,
		},
		{
			Name:           "OCM Controller",
			NamespacedName: types.NamespacedName{Name: "ocm-controller", Namespace: BackplaneOperatorNamespace},
			ResourceType:   &appsv1.Deployment{},
			Expected:       nil,
		},
		{
			Name:           "OCM Proxy Server",
			NamespacedName: types.NamespacedName{Name: "ocm-proxyserver", Namespace: BackplaneOperatorNamespace},
			ResourceType:   &appsv1.Deployment{},
			Expected:       nil,
		},
		{
			Name:           "Cluster Manager Deployment",
			NamespacedName: types.NamespacedName{Name: "cluster-manager", Namespace: BackplaneOperatorNamespace},
			ResourceType:   &appsv1.Deployment{},
			Expected:       nil,
		},
		{
			Name:           "Hive Operator Deployment",
			NamespacedName: types.NamespacedName{Name: "hive-operator", Namespace: BackplaneOperatorNamespace},
<<<<<<< HEAD
<<<<<<< HEAD
=======
>>>>>>> 48f427659e30047f3ced68aa83247af41177a152
			ResourceType:   &appsv1.Deployment{},
			Expected:       nil,
		},
		{
			Name:           "Cluster Manager Deployment",
			NamespacedName: types.NamespacedName{Name: "cluster-manager", Namespace: BackplaneConfigNamespace},
			ResourceType:   &appsv1.Deployment{},
			Expected:       nil,
		},
		{
			Name:           "Hive Operator Deployment",
			NamespacedName: types.NamespacedName{Name: "hive-operator", Namespace: BackplaneConfigNamespace},
=======
>>>>>>>  change backplane config scope and associated code
			ResourceType:   &appsv1.Deployment{},
			Expected:       nil,
		},
		{
			Name:           "Cluster Manager",
			NamespacedName: types.NamespacedName{Name: "cluster-manager"},
			ResourceType:   clusterManager,
			Expected:       nil,
		},
		{
			Name:           "Hive Config",
			NamespacedName: types.NamespacedName{Name: "hive"},
			ResourceType:   hiveConfig,
			Expected:       nil,
		},
	}

	Context("When creating BackplaneConfig", func() {
		It("Should deploy sub components", func() {
			By("By creating a new BackplaneConfig")
			ctx := context.Background()
			backplaneConfig := &v1alpha1.BackplaneConfig{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "backplane.open-cluster-management.io/v1alpha1",
					Kind:       "BackplaneConfig",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      BackplaneConfigName,
					Namespace: BackplaneOperatorNamespace,
				},
				Spec: v1alpha1.BackplaneConfigSpec{},
			}
			Expect(k8sClient.Create(ctx, backplaneConfig)).Should(Succeed())

			for _, test := range tests {
				By(fmt.Sprintf("Ensuring %s is created", test.Name))
				Eventually(func() bool {
					err := k8sClient.Get(ctx, test.NamespacedName, test.ResourceType)
					return err == test.Expected
				}, timeout, interval).Should(BeTrue())
			}
		})

		It("Should indicate resource have been applied", func() {
			key := &v1alpha1.BackplaneConfig{}
			err := k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      BackplaneConfigName,
				Namespace: BackplaneOperatorNamespace,
			}, key)
			Expect(err).To(BeNil())
			Expect(key.Status.Phase).To(Equal(v1alpha1.BackplaneApplied))
		})

		It("Should finalize resources when BackplaneConfig is deleted", func() {
			ctx := context.Background()
			backplaneConfig := &v1alpha1.BackplaneConfig{}
			backplaneConfigLookupKey := types.NamespacedName{Name: BackplaneConfigName}
			err := k8sClient.Get(ctx, backplaneConfigLookupKey, backplaneConfig)
			Expect(err).To(BeNil())
			err = k8sClient.Delete(ctx, backplaneConfig, &client.DeleteOptions{})
			Expect(err).To(BeNil())

			for _, test := range tests {
				By(fmt.Sprintf("Ensuring %s is removed", test.Name))
				Eventually(func() bool {
					err := k8sClient.Get(ctx, test.NamespacedName, test.ResourceType)
					if err != nil && errors.IsNotFound(err) {
						return true
					}
					return false
				}, timeout, interval).Should(BeTrue())
			}
		})
	})
})
