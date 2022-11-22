// Copyright Contributors to the Open Cluster Management project
package status

import (
	"context"
	"fmt"
	"testing"

	"github.com/onsi/gomega"
	v1 "github.com/stolostron/backplane-operator/api/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type buggyClient struct {
	client.Client
	shouldError bool
}

func (cl buggyClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object) error {
	if cl.shouldError {
		return apierrors.NewInternalError(fmt.Errorf("oops"))
	}
	return cl.Client.Get(ctx, key, obj)
}

func TestNewDisabledStatus(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	testNS := types.NamespacedName{Name: "test", Namespace: ""}
	sr := NewDisabledStatus(
		types.NamespacedName{Name: "ns-component", Namespace: ""},
		"the component is disabled",
		[]*unstructured.Unstructured{newUnstructured(testNS, schema.GroupVersionKind{Group: "", Kind: "Namespace", Version: "v1"})},
	)

	g.Expect(sr.GetName()).To(gomega.Equal("ns-component"))
	g.Expect(sr.GetKind()).To(gomega.Equal("Component"))
	g.Expect(sr.GetNamespace()).To(gomega.Equal(""))

	cl := fake.NewClientBuilder().Build()
	condition := sr.Status(cl)
	g.Expect(condition.Available).To(gomega.BeTrue(), "Status should be good because namespace does not exist")

	cl = fake.NewClientBuilder().WithObjects(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test"}}).Build()
	condition = sr.Status(cl)
	g.Expect(condition.Available).To(gomega.BeFalse(), "Status should not be good because namespace exists")

	bc := buggyClient{cl, true}
	condition = sr.Status(bc)
	g.Expect(condition.Type).To(gomega.Equal("Unknown"), "Status should be unknown due to request error")

}

func TestStaticStatus(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	condition := v1.ComponentCondition{
		Type:      "Available",
		Name:      "test-resource",
		Status:    metav1.ConditionFalse,
		Reason:    WaitingForResourceReason,
		Kind:      "Deployment",
		Available: false,
		Message:   "Waiting for namespace",
	}

	static := StaticStatus{
		NamespacedName: types.NamespacedName{Name: "test-resource", Namespace: "test"},
		Kind:           "Deployment",
		Condition:      condition,
	}
	cl := fake.NewClientBuilder().Build()

	g.Expect(static.GetName()).To(gomega.Equal("test-resource"))
	g.Expect(static.GetNamespace()).To(gomega.Equal("test"))
	g.Expect(static.GetKind()).To(gomega.Equal("Deployment"))
	g.Expect(static.Status(cl)).To(gomega.Equal(condition))
}
