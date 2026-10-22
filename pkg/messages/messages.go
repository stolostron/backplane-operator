// Copyright Contributors to the Open Cluster Management project

// Package messages defines user-facing status messages and conditions.
//
// This package contains constant message strings used in status conditions
// and events for the MultiClusterEngine operator.
package messages

const (
	// SkippingExternallyManaged is logged when a component is skipped due to external management
	SkippingExternallyManaged = "Skipping component reconciliation - externally managed"
)
