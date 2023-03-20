// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"context"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	operatorsapiv2 "github.com/operator-framework/api/pkg/operators/v2"
	"github.com/operator-framework/operator-lib/conditions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("OperatorCondition", func() {

	It("valid condition", func() {
		testScheme := scheme.Scheme
		Expect(operatorsapiv2.AddToScheme(testScheme)).Should(Succeed())

		cl := fake.NewClientBuilder().
			WithScheme(testScheme).
			Build()

		GetFactory = func(cl client.Client) conditions.Factory {
			if operatorConditionFactory == nil {
				operatorConditionFactory = OpCondFactoryMock{
					Client: cl,
				}
			}
			return operatorConditionFactory
		}

		oc, err := NewOperatorCondition(cl, "testCondition")
		Expect(err).ShouldNot(HaveOccurred())

		cond, err := oc.cond.Get(context.TODO())
		Expect(err).ShouldNot(HaveOccurred())

		Expect(cond.Type).Should(Equal("testCondition"))

		Expect(
			oc.Set(context.TODO(), metav1.ConditionTrue, "myReason", "my message"),
		).Should(Succeed())

		cond, err = oc.cond.Get(context.TODO())
		Expect(err).ShouldNot(HaveOccurred())
		Expect(cond.Type).Should(Equal("testCondition"))
		Expect(cond.Reason).Should(Equal("myReason"))
		Expect(cond.Message).Should(Equal("my message"))

		oc = &OperatorCondition{}
		msg := UpgradeableAllowMessage
		status := metav1.ConditionTrue
		reason := UpgradeableAllowReason
		ctx := context.Background()
		err = oc.Set(ctx, status, reason, msg)
		Expect(err).ShouldNot(HaveOccurred())

	})
})

func TestOperatorCondition(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Util Suite")
}

var (
	origIsVarSet bool
	origVar      string

	_ = BeforeSuite(func() {
		const OperatorConditionNameEnvVar = "OPERATOR_CONDITION_NAME"
		origVar, origIsVarSet = os.LookupEnv("OperatorConditionNameEnvVar")
	})

	_ = AfterSuite(func() {
		const OperatorConditionNameEnvVar = "OPERATOR_CONDITION_NAME"
		if origIsVarSet {
			os.Setenv(OperatorConditionNameEnvVar, origVar)
		} else {
			os.Unsetenv(OperatorConditionNameEnvVar)
		}
	})
)

type OpCondFactoryMock struct {
	Client client.Client
}

func (fm OpCondFactoryMock) NewCondition(typ operatorsapiv2.ConditionType) (conditions.Condition, error) {
	return &ConditionMock{condition: &metav1.Condition{Type: string(typ)}}, nil
}

func (fm OpCondFactoryMock) GetNamespacedName() (*types.NamespacedName, error) {
	return &types.NamespacedName{Name: "test", Namespace: "default"}, nil
}

type ConditionMock struct {
	condition *metav1.Condition
}

func (c ConditionMock) Get(_ context.Context) (*metav1.Condition, error) {
	return c.condition, nil
}

func (c *ConditionMock) Set(_ context.Context, status metav1.ConditionStatus, options ...conditions.Option) error {
	c.condition.Status = status
	for _, opt := range options {
		opt(c.condition)
	}
	return nil
}
