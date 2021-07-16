// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package deploying

import (
	"context"
	"crypto/sha1"
	"encoding/hex"

	"gopkg.in/yaml.v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

var hashAnnotation = "installer.open-cluster-management.io/last-applied-configuration"

// Deploy attempts to create or update the obj resource depending on whether it exists.
// Returns true if deploy does try to create a new resource
func Deploy(c client.Client, obj *unstructured.Unstructured) (bool, error) {
	var log = zap.New()
	found := &unstructured.Unstructured{}
	found.SetGroupVersionKind(obj.GroupVersionKind())
	err := c.Get(context.TODO(), types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("Creating resource", "Kind", obj.GetKind(), "Name", obj.GetName(), "Namespace", obj.GetNamespace())
			annotate(obj)
			return true, c.Create(context.TODO(), obj)
		}
		return false, err
	}

	// Update if hash doesn't match
	if shasMatch(found, obj) {
		return false, nil
	}
	annotate(obj)

	// If resources exists, update it with current config
	obj.SetResourceVersion(found.GetResourceVersion())
	return false, c.Update(context.TODO(), obj)
}

// annotated modifies a deployment and sets an annotation with the hash of the deployment spec
func annotate(u *unstructured.Unstructured) {
	var log = zap.New().WithValues("Namespace", u.GetNamespace(), "Name", u.GetName())

	hx, err := hash(u)
	if err != nil {
		log.Error(err, "Couldn't marshal deployment spec. Hash not assigned.")
	}

	if anno := u.GetAnnotations(); anno == nil {
		u.SetAnnotations(map[string]string{hashAnnotation: hx})
	} else {
		anno[hashAnnotation] = hx
		u.SetAnnotations(anno)
	}
}

func shasMatch(found, want *unstructured.Unstructured) bool {
	hx, err := hash(want)
	if err != nil {
		zap.New().Error(err, "Couldn't marshal object spec.", "Name", found.GetName())
	}

	if existing := found.GetAnnotations()[hashAnnotation]; existing != hx {
		zap.New().Info("Hashes don't match. Update needed.", "Name", want.GetName(), "Existing sha", existing, "New sha", hx)
		return false
	} else {
		return true
	}
}

func hash(u *unstructured.Unstructured) (string, error) {
	spec, err := yaml.Marshal(u.Object)
	if err != nil {
		return "", err
	}
	h := sha1.New() // #nosec G401 (not using sha for private encryption)
	_, err = h.Write(spec)
	if err != nil {
		return "", err
	}
	bs := h.Sum(nil)
	return hex.EncodeToString(bs), nil
}
