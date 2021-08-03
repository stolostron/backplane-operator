// Copyright Contributors to the Open Cluster Management project
/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	cl "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var (
	backplaneconfiglog = logf.Log.WithName("backplaneconfig-resource")
	Client             cl.Client
)

func (r *BackplaneConfig) SetupWebhookWithManager(mgr ctrl.Manager) error {
	Client = mgr.GetClient()
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
var _ webhook.Defaulter = &BackplaneConfig{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *BackplaneConfig) Default() {
	backplaneconfiglog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-backplane-open-cluster-management-io-v1alpha1-backplaneconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=backplane.open-cluster-management.io,resources=backplaneconfigs,verbs=create;update,versions=v1alpha1,name=vbackplaneconfig.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &BackplaneConfig{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *BackplaneConfig) ValidateCreate() error {
	backplaneconfiglog.Info("validate create", "name", r.Name)

	backplaneConfigList := &BackplaneConfigList{}
	if err := Client.List(context.TODO(), backplaneConfigList); err != nil {
		return fmt.Errorf("unable to list BackplaneConfigs: %s", err)
	}
	if len(backplaneConfigList.Items) == 0 {
		return nil
	}
	// TODO(user): fill in your validation logic upon object creation.
	return errors.New("only 1 backplaneconfig resource may exist")
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *BackplaneConfig) ValidateUpdate(old runtime.Object) error {
	backplaneconfiglog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *BackplaneConfig) ValidateDelete() error {
	backplaneconfiglog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
