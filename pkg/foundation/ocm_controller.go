// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package foundation

import (
	v1alpha1 "github.com/open-cluster-management/backplane-operator/api/v1alpha1"
	"github.com/open-cluster-management/backplane-operator/pkg/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// OCMControllerName is the name of the ocm controller deployment
const OCMControllerName string = "ocm-controller"

// OCMControllerDeployment creates the deployment for the ocm controller
func OCMControllerDeployment(bpc *v1alpha1.BackplaneConfig, overrides map[string]string) *appsv1.Deployment {
	replicas := utils.GetReplicaCount()

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      OCMControllerName,
			Namespace: bpc.Namespace,
			Labels:    defaultLabels(OCMControllerName),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: defaultLabels(OCMControllerName),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: defaultLabels(OCMControllerName),
				},
				Spec: corev1.PodSpec{
					// ImagePullSecrets:   []corev1.LocalObjectReference{{Name: bpc.Spec.ImagePullSecret}},
					ServiceAccountName: ServiceAccount,
					// NodeSelector:       bpc.Spec.NodeSelector,
					Tolerations: defaultTolerations(),
					Affinity:    utils.DistributePods("ocm-antiaffinity-selector", OCMControllerName),
					Containers: []corev1.Container{{
						Image: Image(overrides),
						// ImagePullPolicy: utils.GetImagePullPolicy(bpc),
						Name: OCMControllerName,
						Args: []string{
							"/controller",
						},
						LivenessProbe: &corev1.Probe{
							Handler: corev1.Handler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/healthz",
									Port: intstr.FromInt(8000),
								},
							},
							PeriodSeconds: 10,
						},
						ReadinessProbe: &corev1.Probe{
							Handler: corev1.Handler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/readyz",
									Port: intstr.FromInt(8000),
								},
							},
							PeriodSeconds: 10,
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("2048Mi"),
							},
						},
					}},
				},
			},
		},
	}

	dep.SetOwnerReferences([]metav1.OwnerReference{
		*metav1.NewControllerRef(bpc, bpc.GetObjectKind().GroupVersionKind()),
	})
	return dep
}
