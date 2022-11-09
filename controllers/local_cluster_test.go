// Copyright Contributors to the Open Cluster Management project
package controllers

import (
	"context"
	"errors"
	"testing"

	mcev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/status"
	"github.com/stolostron/backplane-operator/pkg/utils"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testname      = "local-cluster"
	testnamespace = "testnamespace"
)

type localClient struct {
	client.Client
	err string
}

func (lc localClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object) error {
	if lc.err != "" {
		return apierrors.NewInternalError(errors.New(lc.err))
	}
	return lc.Client.Get(ctx, key, obj)
}

func newMCER(c client.Client) *MultiClusterEngineReconciler {
	s := scheme.Scheme
	sm := &status.StatusTracker{
		Client: c,
	}
	return &MultiClusterEngineReconciler{
		Client:        c,
		Scheme:        s,
		StatusManager: sm,
	}
}

func newMC() *unstructured.Unstructured {
	mc := utils.NewManagedCluster()
	mc.SetName(testname)
	mc.SetNamespace(testnamespace)
	return mc
}

type lcTestCase struct {
	desc string
	mc   *unstructured.Unstructured
	err  string
}

func lcTests() []lcTestCase {
	tests := []lcTestCase{}
	tests = append(tests,
		lcTestCase{
			desc: "test",
			mc:   nil,
			err:  "",
		},
	)

	return tests
}

func TestEnsureLocalCluster(t *testing.T) {
	if utils.IsUnitTest() {
		utils.SetUnitTest(false)
		defer func() {
			utils.SetUnitTest(true)
		}()
	}

	builder := fake.NewClientBuilder()
	cl := localClient{
		Client: builder.Build(),
	}

	r := newMCER(cl)
	ctx := context.Background()
	mce := &mcev1.MultiClusterEngine{}

	result, err := r.ensureLocalCluster(ctx, mce)

	if err != nil {
		t.Errorf("r.ensureLocalCluster err: expected nil, got %s", err)
	}

	if result != (ctrl.Result{}) {
		t.Errorf("r.ensureLocalCluster result: expected ctrl.Reult{}, got %#v", result)
	}
}

func TestEnsureNoLocalCluster(t *testing.T) {
	if utils.IsUnitTest() {
		utils.SetUnitTest(false)
		defer func() {
			utils.SetUnitTest(true)
		}()
	}

	builder := fake.NewClientBuilder()
	cl := localClient{
		Client: builder.Build(),
	}

	r := newMCER(cl)
	ctx := context.Background()
	mce := &mcev1.MultiClusterEngine{}

	result, err := r.ensureNoLocalCluster(ctx, mce)

	if err != nil {
		t.Errorf("r.ensureLocalCluster err: expected nil, got %s", err)
	}

	if result != (ctrl.Result{}) {
		t.Errorf("r.ensureLocalCluster result: expected ctrl.Reult{}, got %#v", result)
	}
}
