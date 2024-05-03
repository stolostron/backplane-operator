package v1_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/strings/slices"

	api "github.com/stolostron/backplane-operator/api/v1"
)

func config(name string, enabled bool) api.ComponentConfig {
	return api.ComponentConfig{
		Name:    name,
		Enabled: enabled,
	}
}

func makeMCE(configs ...api.ComponentConfig) *api.MultiClusterEngine {
	mce := &api.MultiClusterEngine{
		Spec: api.MultiClusterEngineSpec{},
	}
	if len(configs) == 0 {
		return mce
	}
	mce.Spec.Overrides = &api.Overrides{
		Components: make([]api.ComponentConfig, len(configs)),
	}
	for i := range configs {
		mce.Spec.Overrides.Components[i] = configs[i]
	}
	return mce
}

var _ = Describe("V1 API Methods", func() {
	Context("when the spec is empty", func() {
		var mce *api.MultiClusterEngine

		BeforeEach(func() {
			mce = makeMCE()
		})

		It("correctly indicates if a component is present", func() {
			Expect(mce.ComponentPresent(api.Discovery)).To(BeFalse())
			Expect(mce.ComponentPresent("test")).To(BeFalse())
		})

		It("correctly indicates if a component is enabled", func() {
			Expect(mce.Enabled(api.Discovery)).To(BeFalse())
		})

		It("enables a component", func() {
			Expect(mce.ComponentPresent(api.Discovery)).To(BeFalse())
			Expect(mce.Enabled(api.Discovery)).To(BeFalse())
			mce.Enable(api.Discovery)
			Expect(mce.ComponentPresent(api.Discovery)).To(BeTrue())
			Expect(mce.Enabled(api.Discovery)).To(BeTrue())
		})

		It("disables a component", func() {
			Expect(mce.ComponentPresent(api.Discovery)).To(BeFalse())
			Expect(mce.Enabled(api.Discovery)).To(BeFalse())
			mce.Disable(api.Discovery)
			Expect(mce.ComponentPresent(api.Discovery)).To(BeTrue())
			Expect(mce.Enabled(api.Discovery)).To(BeFalse())
		})

		It("prunes a component", func() {
			m := makeMCE(config(api.Discovery, true))
			Expect(m.Prune(api.Discovery)).To(BeTrue())
			Expect(m.Prune("test")).To(BeFalse())
		})
	})
})

func TestGetLegacyPrometheusKind(t *testing.T) {
	tests := []struct {
		name  string
		kind  string
		want  int
		want2 []string
	}{
		{
			name:  "legacy Prometheus Configuration Kind",
			kind:  "PrometheusRule",
			want:  2,
			want2: api.LegacyPrometheusKind,
		},
		{
			name:  "legacy Prometheus Configuration Kind",
			kind:  "ServiceMonitor",
			want:  2,
			want2: api.LegacyPrometheusKind,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := api.GetLegacyPrometheusKind()
			if len(got) == 0 {
				t.Errorf("GetLegacyPrometheusKind() = %v, want: %v", len(got), tt.want)
			}

			if ok := slices.Contains(got, tt.kind); !ok {
				t.Errorf("GetLegacyPrometheusKind() = %v, want: %v", got, tt.want2)
			}
		})
	}
}

func TestGetLegacyPrometheusRulesName(t *testing.T) {
	tests := []struct {
		name      string
		component string
		want      string
	}{
		{
			name:      "console PrometheusRule",
			component: api.ConsoleMCE,
			want:      api.MCELegacyPrometheusRules[api.ConsoleMCE],
		},
		{
			name:      "unknown PrometheusRule",
			component: "unknown",
			want:      api.MCELegacyPrometheusRules["unknown"],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := api.GetLegacyPrometheusRulesName(tt.component)
			if err != nil && tt.component != "unknown" {
				t.Errorf("GetLegacyPrometheusRulesName(%v) = %v, want: %v", tt.component, err.Error(), tt.want)
			}

			if got != tt.want {
				t.Errorf("GetLegacyPrometheusRulesName(%v) = %v, want: %v", tt.component, got, tt.want)
			}
		})
	}
}

func TestGetLegacyServiceMonitorName(t *testing.T) {
	tests := []struct {
		name      string
		component string
		want      string
	}{
		{
			name:      "console ServiceMonitor",
			component: api.ConsoleMCE,
			want:      api.MCELegacyServiceMonitors[api.ConsoleMCE],
		},
		{
			name:      "unknown ServiceMonitor",
			component: "unknown",
			want:      api.MCELegacyServiceMonitors["unknown"],
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := api.GetLegacyServiceMonitorName(tt.component)
			if err != nil && tt.component != "unknown" {
				t.Errorf("GetLegacyServiceMonitorName(%v) = %v, want: %v", tt.component, err.Error(), tt.want)
			}

			if got != tt.want {
				t.Errorf("GetLegacyServiceMonitorName(%v) = %v, want: %v", tt.component, got, tt.want)
			}
		})
	}
}

// TODO: put this back later
// func TestHubSizeMarshal(t *testing.T) {
// 	tests := []struct {
// 		name       string
// 		yamlstring string
// 		want       api.HubSize
// 	}{
// 		{
// 			name:       "Marshals when setting hubSize to Large",
// 			yamlstring: `{"hubSize": "Large"}`,
// 			want:       api.Large,
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			var out api.MultiClusterEngineSpec
// 			t.Logf("spec before marshal: %v\n", out)
// 			err := json.Unmarshal([]byte([]byte(tt.yamlstring)), &out)
// 			t.Logf("spec after marshal: %v\n", out)
// 			if err != nil {
// 				t.Errorf("Unable to unmarshal yaml string: %v. %v", tt.yamlstring, err)
// 			}
// 			if out.HubSize != tt.want {
// 				t.Errorf("Hubsize not desired. HubSize: %v, want: %v", out.HubSize, tt.want)
// 			}
// 		})
// 	}
// }
