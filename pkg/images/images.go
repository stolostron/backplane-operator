// Copyright (c) 2021 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package images

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	"github.com/stolostron/backplane-operator/pkg/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

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

// GetImagesWithOverrides gets images from the environment, then updates them based on MCE annotations
func GetImagesWithOverrides(kubeclient client.Client, mce *backplanev1.MultiClusterEngine) (map[string]string, error) {
	// Get images from environment
	images := GetImages()

	// Override image repository if dev annotation present
	if imageRepo := utils.GetImageRepository(mce); imageRepo != "" {
		images = OverrideImageRepository(images, imageRepo)
	}

	// Override individual images if dev configmap present
	if cmName := utils.GetImageOverridesConfigmap(mce); cmName != "" {
		configmap := &corev1.ConfigMap{}
		err := kubeclient.Get(context.TODO(), types.NamespacedName{Name: cmName, Namespace: utils.OperatorNamespace()}, configmap)
		if err != nil {
			return nil, err
		}

		images, err = OverrideImagesWithConfigmap(images, configmap)
		if err != nil {
			return nil, err
		}
	}

	return images, nil
}

// GetImages creates an image map from the environment
func GetImages() map[string]string {
	OperandImagePrefix := "OPERAND_IMAGE_"
	OSBSImagePrefix := "RELATED_IMAGE_"

	images := make(map[string]string)

	// First check for environment variables containing the 'OPERAND_IMAGE_' prefix
	for _, e := range os.Environ() {
		keyValuePair := strings.SplitN(e, "=", 2)
		if strings.HasPrefix(keyValuePair[0], OperandImagePrefix) {
			key := strings.ToLower(strings.Replace(keyValuePair[0], OperandImagePrefix, "", -1))
			images[key] = keyValuePair[1]
		}
	}

	// If entries exist containing operand image prefix, return
	if len(images) > 0 {
		return images
	}

	// If no image overrides found, check 'RELATED_IMAGE_' prefix
	for _, e := range os.Environ() {
		keyValuePair := strings.SplitN(e, "=", 2)
		if strings.HasPrefix(keyValuePair[0], OSBSImagePrefix) {
			key := strings.ToLower(strings.Replace(keyValuePair[0], OSBSImagePrefix, "", -1))
			images[key] = keyValuePair[1]
		}
	}
	return images
}

// OverrideImageRepository updates images with a new repository value
func OverrideImageRepository(images map[string]string, imageRepo string) map[string]string {
	for imageKey, imageRef := range images {
		image := strings.LastIndex(imageRef, "/")
		images[imageKey] = fmt.Sprintf("%s%s", imageRepo, imageRef[image:])
	}
	return images
}

// OverrideImagesWithConfigmap updates an image map with images defined in configmap
func OverrideImagesWithConfigmap(images map[string]string, configmap *corev1.ConfigMap) (map[string]string, error) {
	if len(configmap.Data) != 1 {
		return nil, fmt.Errorf(fmt.Sprintf("Unexpected number of keys in configmap: %s", configmap.Name))
	}

	for _, v := range configmap.Data {
		var manifestImages []ManifestImage
		err := json.Unmarshal([]byte(v), &manifestImages)
		if err != nil {
			return nil, err
		}

		for _, manifestImage := range manifestImages {
			if manifestImage.ImageDigest != "" {
				images[manifestImage.ImageKey] = fmt.Sprintf("%s/%s@%s", manifestImage.ImageRemote, manifestImage.ImageName, manifestImage.ImageDigest)
			} else if manifestImage.ImageTag != "" {
				images[manifestImage.ImageKey] = fmt.Sprintf("%s/%s:%s", manifestImage.ImageRemote, manifestImage.ImageName, manifestImage.ImageTag)
			}

		}
	}
	return images, nil
}
