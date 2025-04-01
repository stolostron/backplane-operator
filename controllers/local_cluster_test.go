// Copyright Contributors to the Open Cluster Management project
package controllers

import (
	"context"
	"errors"
	"os"
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
	testname      = utils.DefaultLocalClusterName
	testnamespace = "testnamespace"
)

func SetUnitTest(on bool) {
	if on {
		os.Setenv(utils.UnitTestEnvVar, "true")
	} else {
		os.Unsetenv(utils.UnitTestEnvVar)
	}
}

type localClient struct {
	client.Client
	err string
}

func (lc localClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
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
	mc := utils.NewManagedCluster(utils.DefaultLocalClusterName)
	mc.SetName(testname)
	mc.SetNamespace(testnamespace)
	return mc
}

func TestEnsureLocalCluster(t *testing.T) {
	if utils.IsUnitTest() {
		SetUnitTest(false)
		defer func() {
			SetUnitTest(true)
		}()
	}

	builder := fake.NewClientBuilder()
	cl := localClient{
		Client: builder.Build(),
	}

	r := newMCER(cl)
	ctx := context.Background()
	mce := &mcev1.MultiClusterEngine{
		Spec: mcev1.MultiClusterEngineSpec{
			LocalClusterName: utils.DefaultLocalClusterName,
		},
	}

	result, err := r.ensureLocalCluster(ctx, mce)

	if err != nil {
		t.Errorf("r.ensureLocalCluster err: expected nil, got %s", err)
	}

	if result != (ctrl.Result{}) {
		t.Errorf("r.ensureLocalCluster result: expected ctrl.Reult{}, got %#v", result)
	}
}

// Case 1: manangedcluster cr and namespace does not exist
func TestEnsureNoLocalCluster1(t *testing.T) {
	if utils.IsUnitTest() {
		SetUnitTest(false)
		defer func() {
			SetUnitTest(true)
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

// Case 2: manangedcluster cr and namespace do exist
func TestEnsureNoLocalCluster2(t *testing.T) {
	if utils.IsUnitTest() {
		SetUnitTest(false)
		defer func() {
			SetUnitTest(true)
		}()
	}

	requeueResult := ctrl.Result{
		RequeueAfter: requeuePeriod,
	}

	ctx := context.Background()
	mce := &mcev1.MultiClusterEngine{
		Spec: mcev1.MultiClusterEngineSpec{
			LocalClusterName: utils.DefaultLocalClusterName,
		},
	}

	builder := fake.NewClientBuilder()
	mc := newMC()
	mc.SetNamespace("")
	ns := utils.NewLocalNamespace(utils.DefaultLocalClusterName)
	builder.WithObjects(mc, ns)
	cl := localClient{
		Client: builder.Build(),
	}

	r := newMCER(cl)

	result, err := r.ensureNoLocalCluster(ctx, mce)
	if err != nil {
		t.Errorf("r.ensureLocalCluster err: expected nil, got %s", err)
	}
	if result != requeueResult {
		t.Errorf("r.ensureLocalCluster result: expected %#v, got %#v", requeueResult, result)
	}

	result, err = r.ensureNoLocalCluster(ctx, mce)

	if err != nil {
		t.Errorf("r.ensureLocalCluster err: expected nil, got %s", err)
	}

	if result != requeueResult {
		t.Errorf("r.ensureLocalCluster result: expected %#v, got %#v", requeueResult, result)
	}
}
