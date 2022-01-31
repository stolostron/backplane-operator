package v1alpha1

const (
	managedServiceAccountEnabledDefault = false
)

func (mce *MultiClusterEngine) ComponentEnabled(c ComponentEnabled) bool {
	if c == ManagedServiceAccount {
		return mce.managedServiceAccountEnabled()
	}
	return false
}

func (mce *MultiClusterEngine) hasComponentConfig() bool {
	return mce.Spec.ComponentConfig != nil
}

func (mce *MultiClusterEngine) hasManagedServiceAccountConfig() bool {
	if !mce.hasComponentConfig() {
		return false
	}
	return mce.Spec.ComponentConfig.ManagedServiceAccount != nil
}

func (mce *MultiClusterEngine) managedServiceAccountEnabled() bool {
	if !mce.hasManagedServiceAccountConfig() {
		return managedServiceAccountEnabledDefault
	}
	return mce.Spec.ComponentConfig.ManagedServiceAccount.Enable
}
