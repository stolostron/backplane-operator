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
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	client "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("BackplaneConfig controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		BackplaneConfigName        = "test-backplaneconfig"
		BackplaneOperatorNamespace = "default"
		DestinationNamespace       = "test"
		JobName                    = "test-job"

		timeout  = time.Second * 60
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	clusterManager := &unstructured.Unstructured{}
	clusterManager.SetGroupVersionKind(schema.GroupVersionKind{Group: "operator.open-cluster-management.io", Version: "v1", Kind: "ClusterManager"})
	hiveConfig := &unstructured.Unstructured{}
	hiveConfig.SetGroupVersionKind(schema.GroupVersionKind{Group: "hive.openshift.io", Version: "v1", Kind: "HiveConfig"})
	os.Setenv("POD_NAMESPACE", "default")

	tests := []struct {
		Name           string
		NamespacedName types.NamespacedName
		ResourceType   client.Object
		Expected       error
	}{
		{
			Name:           "Backplane Config",
			NamespacedName: types.NamespacedName{Name: BackplaneConfigName},
			ResourceType:   &v1alpha1.MultiClusterEngine{},
			Expected:       nil,
		},
		{
			Name:           "OCM Webhook",
			NamespacedName: types.NamespacedName{Name: "ocm-webhook", Namespace: DestinationNamespace},
			ResourceType:   &appsv1.Deployment{},
			Expected:       nil,
		},
		{
			Name:           "OCM Controller",
			NamespacedName: types.NamespacedName{Name: "ocm-controller", Namespace: DestinationNamespace},
			ResourceType:   &appsv1.Deployment{},
			Expected:       nil,
		},
		{
			Name:           "OCM Proxy Server",
			NamespacedName: types.NamespacedName{Name: "ocm-proxyserver", Namespace: DestinationNamespace},
			ResourceType:   &appsv1.Deployment{},
			Expected:       nil,
		},
		{
			Name:           "Cluster Manager Deployment",
			NamespacedName: types.NamespacedName{Name: "cluster-manager", Namespace: DestinationNamespace},
			ResourceType:   &appsv1.Deployment{},
			Expected:       nil,
		},
		{
			Name:           "Hive Operator Deployment",
			NamespacedName: types.NamespacedName{Name: "hive-operator", Namespace: DestinationNamespace},
			ResourceType:   &appsv1.Deployment{},
			Expected:       nil,
		},
		{
			Name:           "Managed Cluster Import Controller",
			NamespacedName: types.NamespacedName{Name: "managedcluster-import-controller-v2", Namespace: DestinationNamespace},
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
			backplaneConfig := &v1alpha1.MultiClusterEngine{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "multicluster.openshift.io/v1alpha1",
					Kind:       "MultiClusterEngine",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: BackplaneConfigName,
				},
				Spec: v1alpha1.MultiClusterEngineSpec{
					TargetNamespace: DestinationNamespace,
					ImagePullSecret: "testsecret",
				},
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

		It("Should check for ownerreference on resources", func() {
			ctx := context.Background()
			for i, test := range tests {
				if i == 0 {
					continue // config itself won't have ownerreference
				}
				By(fmt.Sprintf("Ensuring %s has an ownerreference set", test.Name))
				Eventually(func(g Gomega) {
					g.Expect(k8sClient.Get(ctx, test.NamespacedName, test.ResourceType)).To(Succeed())
					g.Expect(len(test.ResourceType.GetOwnerReferences())).To(Equal(1), fmt.Sprintf("Missing ownerreference on %s", test.Name))
					g.Expect(test.ResourceType.GetOwnerReferences()[0].Name).To(Equal(BackplaneConfigName))
				}, timeout, interval).Should(Succeed())
			}
		})
	})
	os.Unsetenv("POD_NAMESPACE")
})
