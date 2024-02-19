// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

// CacheSpec ...
type CacheSpec struct {
	ImageOverrides      map[string]string
	ImageOverridesCM    string
	ImageRepository     string
	TemplateOverrides   map[string]string
	TemplateOverridesCM string
}
