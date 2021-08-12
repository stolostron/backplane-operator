// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package backplane_install_test

import (
	"context"
	// "fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	// corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"

	backplane "github.com/open-cluster-management/backplane-operator/api/v1alpha1"

	// apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	// "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

const (
	BackplaneConfigName = "backplane"
	timeout             = time.Second * 10
	duration            = time.Second * 1
	interval            = time.Millisecond * 250
)

var (
	ctx                = context.Background()
	globalsInitialized = false
	backplaneNamespace = ""
	baseURL            = ""

	k8sClient client.Client

	backplaneConfig = types.NamespacedName{}
)

func initializeGlobals() {
	backplaneNamespace = *BackplaneNamespace
	// baseURL = *BaseURL
	backplaneConfig = types.NamespacedName{
		Name:      BackplaneConfigName,
		Namespace: backplaneNamespace,
	}
}

var _ = Describe("This is a test", func() {

	BeforeEach(func() {
		if !globalsInitialized {
			initializeGlobals()
			globalsInitialized = true
		}
	})

	Context("Creating a BackplaneConfig", func() {
		It("Should install all components ", func() {
			By("By creating a new BackplaneConfig", func() {
				Expect(k8sClient.Create(ctx, defaultBackplaneConfig())).Should(Succeed())

			})
		})
		It("Should check that all components were installed correctly", func() {
			By("Ensuring the status of BackplaneConfig", func() {

				Eventually(func() bool {
					key := &backplane.BackplaneConfig{}
					k8sClient.Get(context.Background(), types.NamespacedName{
						Name:      BackplaneConfigName,
						Namespace: backplaneNamespace,
					}, key)
					return key.Status.Phase == backplane.BackplaneApplied
				}, timeout, interval).Should(BeTrue())

			})
		})
	})
})

func defaultBackplaneConfig() *backplane.BackplaneConfig {
	return &backplane.BackplaneConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      BackplaneConfigName,
			Namespace: backplaneNamespace,
		},
		Spec: backplane.BackplaneConfigSpec{
			Foo: "bar",
		},
		Status: backplane.BackplaneConfigStatus{
			Phase: "",
		},
	}
}
