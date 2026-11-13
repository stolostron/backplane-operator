// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	backplanev1 "github.com/stolostron/backplane-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestEnsureNetworkPolicyCreatesManagedEgressRules(t *testing.T) {
	ctx := context.Background()
	rules := []networkingv1.NetworkPolicyEgressRule{
		egressRule(3128, "192.0.2.10/32"),
		egressRule(22, "0.0.0.0/0"),
	}
	reconciler := networkPolicyTestReconciler()
	template := networkPolicyTemplate("multicluster-engine", "test-network-policy", rules, true)

	if err := reconciler.ensureNetworkPolicy(ctx, networkPolicyTestMCE(), template, testNetworkPolicyLog()); err != nil {
		t.Fatalf("ensureNetworkPolicy() error = %v", err)
	}

	created := &networkingv1.NetworkPolicy{}
	if err := reconciler.Client.Get(ctx, types.NamespacedName{Namespace: "multicluster-engine", Name: "test-network-policy"}, created); err != nil {
		t.Fatalf("failed to get created NetworkPolicy: %v", err)
	}
	if len(created.Spec.Egress) != len(rules) {
		t.Fatalf("egress rule count = %d, want %d", len(created.Spec.Egress), len(rules))
	}
	assertManagedEgressAnnotation(t, created, rules)
}

func TestEnsureNetworkPolicyPreservesOperatorRulesAndRemovesStaleRules(t *testing.T) {
	ctx := context.Background()
	oldManaged := []networkingv1.NetworkPolicyEgressRule{egressRule(8080, "198.51.100.10/32")}
	desired := []networkingv1.NetworkPolicyEgressRule{egressRule(3128, "192.0.2.10/32")}
	operator := egressRule(443, "203.0.113.10/32")
	existing := networkPolicy("multicluster-engine", "test-network-policy", append([]networkingv1.NetworkPolicyEgressRule{oldManaged[0], operator}, desired...), true)
	setNetworkPolicyAnnotation(t, existing, oldManaged)
	reconciler := networkPolicyTestReconciler(existing)
	template := networkPolicyTemplate("multicluster-engine", "test-network-policy", desired, true)

	if err := reconciler.ensureNetworkPolicy(ctx, networkPolicyTestMCE(), template, testNetworkPolicyLog()); err != nil {
		t.Fatalf("ensureNetworkPolicy() error = %v", err)
	}

	updated := &networkingv1.NetworkPolicy{}
	if err := reconciler.Client.Get(ctx, types.NamespacedName{Namespace: "multicluster-engine", Name: "test-network-policy"}, updated); err != nil {
		t.Fatalf("failed to get updated NetworkPolicy: %v", err)
	}
	if containsEgressRule(updated.Spec.Egress, oldManaged[0]) {
		t.Fatal("stale managed egress rule was not removed")
	}
	if !containsEgressRule(updated.Spec.Egress, operator) {
		t.Fatal("operator-added egress rule was not preserved")
	}
	if !containsEgressRule(updated.Spec.Egress, desired[0]) {
		t.Fatal("desired egress rule was not added")
	}
	assertManagedEgressAnnotation(t, updated, nil)
}

func TestEnsureNetworkPolicyDoesNotClaimOperatorRuleMatchingDesiredRule(t *testing.T) {
	ctx := context.Background()
	desired := []networkingv1.NetworkPolicyEgressRule{egressRule(3128, "192.0.2.10/32")}
	existing := networkPolicy("multicluster-engine", "test-network-policy", desired, true)
	reconciler := networkPolicyTestReconciler(existing)
	template := networkPolicyTemplate("multicluster-engine", "test-network-policy", desired, true)

	if err := reconciler.ensureNetworkPolicy(ctx, networkPolicyTestMCE(), template, testNetworkPolicyLog()); err != nil {
		t.Fatalf("ensureNetworkPolicy() error = %v", err)
	}

	updated := &networkingv1.NetworkPolicy{}
	if err := reconciler.Client.Get(ctx, types.NamespacedName{Namespace: "multicluster-engine", Name: "test-network-policy"}, updated); err != nil {
		t.Fatalf("failed to get updated NetworkPolicy: %v", err)
	}
	assertManagedEgressAnnotation(t, updated, nil)

	removedTemplate := networkPolicyTemplate("multicluster-engine", "test-network-policy", nil, true)
	if err := reconciler.ensureNetworkPolicy(ctx, networkPolicyTestMCE(), removedTemplate, testNetworkPolicyLog()); err != nil {
		t.Fatalf("second ensureNetworkPolicy() error = %v", err)
	}
	if err := reconciler.Client.Get(ctx, types.NamespacedName{Namespace: "multicluster-engine", Name: "test-network-policy"}, updated); err != nil {
		t.Fatalf("failed to get final NetworkPolicy: %v", err)
	}
	if !containsEgressRule(updated.Spec.Egress, desired[0]) {
		t.Fatal("operator-added egress rule was removed after it stopped matching a desired rule")
	}
}

func TestEnsureNetworkPolicyMigratesLegacyManagedRules(t *testing.T) {
	ctx := context.Background()
	legacy := []networkingv1.NetworkPolicyEgressRule{egressRule(8080, "198.51.100.10/32")}
	desired := []networkingv1.NetworkPolicyEgressRule{egressRule(3128, "192.0.2.10/32")}
	operator := egressRule(443, "203.0.113.10/32")
	existing := networkPolicy("multicluster-engine", "test-network-policy", []networkingv1.NetworkPolicyEgressRule{legacy[0], operator}, true)
	reconciler := networkPolicyTestReconciler(existing)
	template := networkPolicyTemplate("multicluster-engine", "test-network-policy", desired, true)
	setTemplateAnnotation(t, template, legacyManagedEgressRulesAnnotation, legacy)

	if err := reconciler.ensureNetworkPolicy(ctx, networkPolicyTestMCE(), template, testNetworkPolicyLog()); err != nil {
		t.Fatalf("ensureNetworkPolicy() error = %v", err)
	}

	updated := &networkingv1.NetworkPolicy{}
	if err := reconciler.Client.Get(ctx, types.NamespacedName{Namespace: "multicluster-engine", Name: "test-network-policy"}, updated); err != nil {
		t.Fatalf("failed to get updated NetworkPolicy: %v", err)
	}
	if containsEgressRule(updated.Spec.Egress, legacy[0]) {
		t.Fatal("legacy managed egress rule was not removed")
	}
	if !containsEgressRule(updated.Spec.Egress, operator) || !containsEgressRule(updated.Spec.Egress, desired[0]) {
		t.Fatal("migration did not preserve operator rules and add desired rules")
	}
	assertManagedEgressAnnotation(t, updated, desired)
	if _, exists := updated.Annotations[legacyManagedEgressRulesAnnotation]; exists {
		t.Fatal("legacy managed egress annotation was persisted")
	}

	removedTemplate := networkPolicyTemplate("multicluster-engine", "test-network-policy", nil, true)
	if err := reconciler.ensureNetworkPolicy(ctx, networkPolicyTestMCE(), removedTemplate, testNetworkPolicyLog()); err != nil {
		t.Fatalf("second ensureNetworkPolicy() error = %v", err)
	}
	if err := reconciler.Client.Get(ctx, types.NamespacedName{Namespace: "multicluster-engine", Name: "test-network-policy"}, updated); err != nil {
		t.Fatalf("failed to get final NetworkPolicy: %v", err)
	}
	if containsEgressRule(updated.Spec.Egress, desired[0]) {
		t.Fatal("migrated managed rule was not removed after it left the template")
	}
	if !containsEgressRule(updated.Spec.Egress, operator) {
		t.Fatal("operator-added egress rule was not preserved after managed-rule removal")
	}
}

func TestEnsureNetworkPolicyRejectsInvalidManagedEgressAnnotation(t *testing.T) {
	ctx := context.Background()
	existing := networkPolicy("multicluster-engine", "test-network-policy", nil, true)
	existing.Annotations[managedEgressRulesAnnotation] = "{invalid"
	reconciler := networkPolicyTestReconciler(existing)
	template := networkPolicyTemplate("multicluster-engine", "test-network-policy", nil, true)

	err := reconciler.ensureNetworkPolicy(ctx, networkPolicyTestMCE(), template, testNetworkPolicyLog())
	if err == nil {
		t.Fatal("ensureNetworkPolicy() error = nil, want invalid annotation error")
	}
}

func TestEnsureNetworkPolicySkipsUnownedNetworkPolicy(t *testing.T) {
	ctx := context.Background()
	existingRules := []networkingv1.NetworkPolicyEgressRule{egressRule(443, "203.0.113.10/32")}
	existing := networkPolicy("multicluster-engine", "test-network-policy", existingRules, false)
	reconciler := networkPolicyTestReconciler(existing)
	template := networkPolicyTemplate("multicluster-engine", "test-network-policy", []networkingv1.NetworkPolicyEgressRule{egressRule(3128, "192.0.2.10/32")}, true)

	if err := reconciler.ensureNetworkPolicy(ctx, networkPolicyTestMCE(), template, testNetworkPolicyLog()); err != nil {
		t.Fatalf("ensureNetworkPolicy() error = %v", err)
	}

	updated := &networkingv1.NetworkPolicy{}
	if err := reconciler.Client.Get(ctx, types.NamespacedName{Namespace: "multicluster-engine", Name: "test-network-policy"}, updated); err != nil {
		t.Fatalf("failed to get NetworkPolicy: %v", err)
	}
	if len(updated.Spec.Egress) != len(existingRules) {
		t.Fatalf("egress rule count = %d, want %d", len(updated.Spec.Egress), len(existingRules))
	}
	if _, exists := updated.Annotations[managedEgressRulesAnnotation]; exists {
		t.Fatal("unowned NetworkPolicy was modified")
	}
}

func TestEqualEgressRulesTreatsNilAndEmptySlicesAsEqual(t *testing.T) {
	if !equalEgressRules(nil, []networkingv1.NetworkPolicyEgressRule{}) {
		t.Fatal("nil and empty egress rules are not equal")
	}
	if !equalEgressRules([]networkingv1.NetworkPolicyEgressRule{}, nil) {
		t.Fatal("empty and nil egress rules are not equal")
	}
}

func TestEnsureNetworkPolicyDoesNotUpdateUnchangedEmptyEgressRules(t *testing.T) {
	ctx := context.Background()
	existing := networkPolicy("multicluster-engine", "test-network-policy", nil, true)
	existing.Annotations[managedEgressRulesAnnotation] = "[]"
	updateCount := 0
	reconciler := MultiClusterEngineReconciler{
		Client: fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(existing).WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				updateCount++
				return c.Update(ctx, obj, opts...)
			},
		}).Build(),
	}
	template := networkPolicyTemplate("multicluster-engine", "test-network-policy", nil, true)

	if err := reconciler.ensureNetworkPolicy(ctx, networkPolicyTestMCE(), template, testNetworkPolicyLog()); err != nil {
		t.Fatalf("ensureNetworkPolicy() error = %v", err)
	}
	if updateCount != 0 {
		t.Fatalf("Update call count = %d, want 0", updateCount)
	}
}

func networkPolicyTestMCE() *backplanev1.MultiClusterEngine {
	return &backplanev1.MultiClusterEngine{
		ObjectMeta: metav1.ObjectMeta{Name: "test-mce"},
		Spec:       backplanev1.MultiClusterEngineSpec{TargetNamespace: "multicluster-engine"},
	}
}

func networkPolicyTestReconciler(objects ...client.Object) MultiClusterEngineReconciler {
	return MultiClusterEngineReconciler{
		Client: fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(objects...).Build(),
	}
}

func testNetworkPolicyLog() logr.Logger {
	return ctrl.Log.WithName("network-policy-test")
}

func networkPolicyTemplate(namespace, name string, rules []networkingv1.NetworkPolicyEgressRule, owned bool) *unstructured.Unstructured {
	policy := networkPolicy(namespace, name, rules, owned)
	object, err := runtime.DefaultUnstructuredConverter.ToUnstructured(policy)
	if err != nil {
		panic(err)
	}
	return &unstructured.Unstructured{Object: object}
}

func networkPolicy(namespace, name string, rules []networkingv1.NetworkPolicyEgressRule, owned bool) *networkingv1.NetworkPolicy {
	labels := map[string]string{}
	if owned {
		labels["backplaneconfig.name"] = "test-mce"
	}
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      labels,
			Annotations: map[string]string{},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress:      rules,
		},
	}
}

func egressRule(port int32, cidr string) networkingv1.NetworkPolicyEgressRule {
	protocol := corev1.ProtocolTCP
	return networkingv1.NetworkPolicyEgressRule{
		Ports: []networkingv1.NetworkPolicyPort{{Protocol: &protocol, Port: intstrPtr(port)}},
		To: []networkingv1.NetworkPolicyPeer{{
			IPBlock: &networkingv1.IPBlock{CIDR: cidr},
		}},
	}
}

func intstrPtr(value int32) *intstr.IntOrString {
	return &intstr.IntOrString{Type: intstr.Int, IntVal: value}
}

func setNetworkPolicyAnnotation(t *testing.T, policy *networkingv1.NetworkPolicy, rules []networkingv1.NetworkPolicyEgressRule) {
	t.Helper()
	encoded, err := json.Marshal(rules)
	if err != nil {
		t.Fatalf("failed to encode managed rules: %v", err)
	}
	policy.Annotations[managedEgressRulesAnnotation] = string(encoded)
}

func setTemplateAnnotation(
	t *testing.T,
	template *unstructured.Unstructured,
	annotation string,
	rules []networkingv1.NetworkPolicyEgressRule,
) {
	t.Helper()
	encoded, err := json.Marshal(rules)
	if err != nil {
		t.Fatalf("failed to encode annotation rules: %v", err)
	}
	annotations := template.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[annotation] = string(encoded)
	template.SetAnnotations(annotations)
}

func assertManagedEgressAnnotation(t *testing.T, policy *networkingv1.NetworkPolicy, rules []networkingv1.NetworkPolicyEgressRule) {
	t.Helper()
	value, exists := policy.Annotations[managedEgressRulesAnnotation]
	if !exists {
		t.Fatal("managed egress rules annotation is missing")
	}
	decoded := []networkingv1.NetworkPolicyEgressRule{}
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		t.Fatalf("failed to decode managed rules annotation: %v", err)
	}
	if len(decoded) != len(rules) {
		t.Fatalf("managed rule count = %d, want %d", len(decoded), len(rules))
	}
	for _, rule := range rules {
		if !containsEgressRule(decoded, rule) {
			t.Fatal("managed egress rules annotation does not contain a desired rule")
		}
	}
}
