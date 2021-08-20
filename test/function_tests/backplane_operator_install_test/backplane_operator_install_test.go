// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package backplane_install_test

import (
	"context"
	// "fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	// corev1 "k8s.io/api/core/v1"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	backplane "github.com/open-cluster-management/backplane-operator/api/v1alpha1"

	// apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	// "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

const (
	BackplaneConfigName = "backplane"
	installTimeout      = time.Minute * 5
	duration            = time.Second * 1
	interval            = time.Millisecond * 250
)

var (
	ctx                = context.Background()
	globalsInitialized = false
	baseURL            = ""

	k8sClient client.Client

	backplaneConfig = types.NamespacedName{}
)

func initializeGlobals() {
	// baseURL = *BaseURL
	backplaneConfig = types.NamespacedName{
		Name: BackplaneConfigName,
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
			By("Ensuring the BackplaneConfig becomes available", func() {
				Eventually(func() bool {
					key := &backplane.BackplaneConfig{}
					k8sClient.Get(context.Background(), types.NamespacedName{
						Name: BackplaneConfigName,
					}, key)
					return key.Status.Phase == backplane.BackplanePhaseAvailable
				}, installTimeout, interval).Should(BeTrue())

			})
		})

		It("Should check for a healthy status", func() {
			config := &backplane.BackplaneConfig{}
			Expect(k8sClient.Get(ctx, backplaneConfig, config)).To(Succeed())

			By("Checking the phase", func() {
				Expect(config.Status.Phase).To(Equal(backplane.BackplanePhaseAvailable))
			})
			By("Checking the components", func() {
				Expect(len(config.Status.Components)).Should(BeNumerically(">=", 6), "Expected at least 6 components in status")
			})
			By("Checking the conditions", func() {
				available := backplane.BackplaneCondition{}
				for _, c := range config.Status.Conditions {
					if c.Type == backplane.BackplaneAvailable {
						available = c
					}
				}
				Expect(available.Status).To(Equal(metav1.ConditionTrue))
			})
		})
	})
})

func defaultBackplaneConfig() *backplane.BackplaneConfig {
	return &backplane.BackplaneConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: BackplaneConfigName,
		},
		Spec: backplane.BackplaneConfigSpec{
			Foo: "bar",
		},
		Status: backplane.BackplaneConfigStatus{
			Phase: "",
		},
	}
}
