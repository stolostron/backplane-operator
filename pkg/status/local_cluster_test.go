// Copyright Contributors to the Open Cluster Management project
package status

import (
	"context"
	"errors"
	"testing"

	"github.com/stolostron/backplane-operator/pkg/utils"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
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

func (lc localClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	if lc.err != "" {
		return apierrors.NewInternalError(errors.New(lc.err))
	}
	return lc.Client.Get(ctx, key, obj)
}

func newLCS() *LocalClusterStatus {
	return &LocalClusterStatus{
		NamespacedName: types.NamespacedName{
			Name:      testname,
			Namespace: testnamespace,
		},
	}
}

func newMC() *unstructured.Unstructured {
	mc := utils.NewManagedCluster(testname)
	mc.SetName(testname)
	mc.SetNamespace(testnamespace)
	return mc
}

func mcNotReady() *unstructured.Unstructured {
	mc := newMC()
	mc.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"type":    Accepted,
				"reason":  Accepted,
				"message": Accepted,
				"status":  "true",
			},
		},
	}
	return mc
}

func mcReady() *unstructured.Unstructured {
	mc := newMC()
	mc.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"type":    Accepted,
				"reason":  Accepted,
				"message": Accepted,
				"status":  "true",
			},
			map[string]interface{}{
				"type":    Joined,
				"reason":  Joined,
				"message": Joined,
				"status":  "true",
			},
			map[string]interface{}{
				"type":    Available,
				"reason":  Available,
				"message": Available,
				"status":  "true",
			},
		},
	}
	return mc
}

func TestGet(t *testing.T) {
	lcs := newLCS()
	if n := lcs.GetName(); n != testname {
		t.Errorf("lcs.GetName: expected %q, got %q", testname, n)
	}
	if ns := lcs.GetNamespace(); ns != testnamespace {
		t.Errorf("lcs.GetNamespace: expected %q, got %q", testnamespace, ns)
	}
	if k := lcs.GetKind(); k != lcsKind {
		t.Errorf("lcs.GetKind: expected %q, got %q", lcsKind, k)
	}
}

type lcsTestCase struct {
	desc      string
	enabled   bool
	mc        *unstructured.Unstructured
	err       string
	available bool
	reason    string
}

func lcsTests() []lcsTestCase {
	tests := []lcsTestCase{}
	tests = append(tests,
		lcsTestCase{
			desc:      "enabled, client error",
			enabled:   true,
			mc:        nil,
			err:       "client error",
			available: false,
			reason:    "No conditions available",
		},
		lcsTestCase{
			desc:      "enabled, mc not ready",
			enabled:   true,
			mc:        mcNotReady(),
			err:       "",
			available: false,
			reason:    "HubAcceptedManagedCluster",
		},
		lcsTestCase{
			desc:      "enabled, mc ready",
			enabled:   true,
			mc:        mcReady(),
			err:       "",
			available: true,
			reason:    MCImported,
		},
		lcsTestCase{
			desc:      "enabled, no mc",
			enabled:   true,
			mc:        nil,
			err:       "",
			available: false,
			reason:    "No conditions available",
		},
		lcsTestCase{
			desc:      "disabled, no mc",
			enabled:   false,
			mc:        nil,
			err:       "",
			available: true,
			reason:    MCDisabled,
		},
		lcsTestCase{
			desc:      "disabled, with mc",
			enabled:   false,
			mc:        newMC(),
			err:       "",
			available: false,
			reason:    MCDisabled,
		},
	)

	return tests
}

func TestStatus(t *testing.T) {
	for _, test := range lcsTests() {
		builder := fake.NewClientBuilder()
		if test.mc != nil {
			builder.WithObjects(test.mc)
		}
		cl := localClient{
			Client: builder.Build(),
			err:    test.err,
		}

		lcs := newLCS()
		lcs.Enabled = test.enabled
		status := lcs.Status(cl)

		if status.Reason != test.reason {
			t.Errorf("lcs.Status %s: expected reason %q, got %q", test.desc, test.reason, status.Reason)
		}
		if status.Available != test.available {
			t.Errorf("lcs.Status %s: expected available %t, got %t",
				test.desc, test.available, status.Available,
			)
		}
	}
}
