// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package manifest

// ManifestImage contains details for a specific image version
type ManifestImage struct {
	ImageKey     string `json:"image-key"`
	ImageName    string `json:"image-name"`
	ImageVersion string `json:"image-version"`

	// remote registry where image is stored
	ImageRemote string `json:"image-remote"`

	// immutable sha version identifier
	ImageDigest string `json:"image-digest"`

	ImageTag string `json:"image-tag"`
}

type ManifestTemplate struct {
	TemplateOverrides map[string]interface{} `json:"templateOverrides" yaml:"templateOverrides"`
}
