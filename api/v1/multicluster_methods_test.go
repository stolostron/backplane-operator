package v1_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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
	})
})
