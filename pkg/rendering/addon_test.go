// Copyright Contributors to the Open Cluster Management project

package renderer

import (
	"testing"

	v1 "github.com/stolostron/backplane-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestRenderHypershiftAddon(t *testing.T) {
	mce := &v1.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-mce",
		},
		Spec: v1.MultiClusterEngineSpec{},
	}
	t.Run("Adds MCE labels to resource", func(t *testing.T) {
		got, err := RenderHypershiftAddon(mce)
		if err != nil {
			t.Errorf("RenderHypershiftAddon() error = %v, wantErr %v", err, nil)
			return
		}
		if got.GetLabels()["backplaneconfig.name"] != mce.Name {
			t.Errorf("RenderHypershiftAddon() did not return a resouce with MCE labels")
		}
	})
}
