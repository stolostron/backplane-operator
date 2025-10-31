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

package utils

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	// ctrl "sigs.k8s.io/controller-runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	"open-cluster-management.io/sdk-go/pkg/servingcert"

	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment
var ctx context.Context
var cancel context.CancelFunc

var _ = Describe("Non-OCP cert management", func() {
	It("creating certs from scratch", func() {
		ctx, cancel = context.WithCancel(context.Background())

		testEnv = &envtest.Environment{
			CRDDirectoryPaths: []string{},
			CRDInstallOptions: envtest.CRDInstallOptions{
				CleanUpAfterUse: true,
			},
			ErrorIfCRDPathMissing: false,
		}

		cfg, err := testEnv.Start()
		Expect(err).NotTo(HaveOccurred())

		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient).NotTo(BeNil())
		err = DetectOpenShift(k8sClient)
		Expect(err).ToNot(HaveOccurred())

		kubeClient, err := kubernetes.NewForConfig(cfg)
		Expect(err).ToNot(HaveOccurred())

		err = k8sClient.Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
			},
			Spec: corev1.NamespaceSpec{},
		})
		Expect(err).ToNot(HaveOccurred())

		// Don't pre-create the secret or configmap - let the ServingCertController create them
		// This tests that the controller can properly initialize and populate certificates from scratch

		NewGlobalServingCertCABundleGetter(kubeClient, servingcert.DefaultCABundleConfigmapName, "test")

		servingcert.NewServingCertController("test", kubeClient).
			WithTargetServingCerts([]servingcert.TargetServingCertOptions{
				{
					Name:      "multicluster-engine-operator-webhook",
					HostNames: []string{fmt.Sprintf("multicluster-engine-operator-webhook-service.%s.svc", "test")},
					// LoadDir:   "/tmp/k8s-webhook-server/serving-certs",
				},
				{
					Name:      "ocm-webhook",
					HostNames: []string{fmt.Sprintf("ocm-webhook.%s.svc", "test")},
				},
			}).Start(ctx)

		// Wait for ServingCertController to populate the CA bundle configmap asynchronously
		Eventually(func() error {
			_, err := GetServingCertCABundle()
			return err
		}, "10s", "1s").Should(Succeed())

		// Wait for ServingCertController to populate the secret asynchronously
		Eventually(func() error {
			return DumpServingCertSecret()
		}, "10s", "1s").Should(Succeed())

	})

	It("managing pre-existing certs", func() {
		ctx, cancel = context.WithCancel(context.Background())

		testEnv = &envtest.Environment{
			CRDDirectoryPaths: []string{},
			CRDInstallOptions: envtest.CRDInstallOptions{
				CleanUpAfterUse: true,
			},
			ErrorIfCRDPathMissing: false,
		}

		cfg, err := testEnv.Start()
		Expect(err).NotTo(HaveOccurred())

		k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient).NotTo(BeNil())
		err = DetectOpenShift(k8sClient)
		Expect(err).ToNot(HaveOccurred())

		kubeClient, err := kubernetes.NewForConfig(cfg)
		Expect(err).ToNot(HaveOccurred())

		err = k8sClient.Create(context.Background(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test2",
			},
			Spec: corev1.NamespaceSpec{},
		})
		Expect(err).ToNot(HaveOccurred())

		// Pre-create the secret and configmap to simulate user-provided resources
		// This tests that the controller can properly reconcile and populate pre-existing certificates
		err = k8sClient.Create(context.Background(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multicluster-engine-operator-webhook",
				Namespace: "test2",
			},
			Data: map[string][]byte{
				"ca.crt":  []byte(""),
				"tls.key": []byte(""),
				"tls.crt": []byte(""),
			},
		})
		Expect(err).ToNot(HaveOccurred())

		err = k8sClient.Create(context.Background(), &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      servingcert.DefaultCABundleConfigmapName,
				Namespace: "test2",
			},
			Data: map[string]string{
				"ca-bundle.crt": "",
			},
		})
		Expect(err).ToNot(HaveOccurred())

		NewGlobalServingCertCABundleGetter(kubeClient, servingcert.DefaultCABundleConfigmapName, "test2")

		servingcert.NewServingCertController("test2", kubeClient).
			WithTargetServingCerts([]servingcert.TargetServingCertOptions{
				{
					Name:      "multicluster-engine-operator-webhook",
					HostNames: []string{fmt.Sprintf("multicluster-engine-operator-webhook-service.%s.svc", "test2")},
				},
				{
					Name:      "ocm-webhook",
					HostNames: []string{fmt.Sprintf("ocm-webhook.%s.svc", "test2")},
				},
			}).Start(ctx)

		// Wait for ServingCertController to populate the CA bundle configmap asynchronously
		Eventually(func() error {
			_, err := GetServingCertCABundle()
			return err
		}, "10s", "1s").Should(Succeed())

		// Wait for ServingCertController to populate the secret asynchronously
		Eventually(func() error {
			return DumpServingCertSecret()
		}, "10s", "1s").Should(Succeed())

	})
	It("checking other functions", func() {
		component := ComponentOnNonOCP("cluster-manager")
		Expect(component).To(Equal(true))
		component = ComponentOnNonOCP("discovery")
		Expect(component).To(Equal(false))
	})
})
