package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	v1alpha1 "github.com/astradns/astradns-types/api/v1alpha1"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestDNSUpstreamPoolUniquenessWebhookAllowsFirstPool(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	webhook := &DNSUpstreamPoolUniquenessWebhook{Client: client}

	pool := &v1alpha1.DNSUpstreamPool{
		TypeMeta: metav1.TypeMeta{APIVersion: "dns.astradns.com/v1alpha1", Kind: "DNSUpstreamPool"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pool-a",
			Namespace: "test-ns",
		},
	}

	response := webhook.Handle(context.Background(), newCreateRequest(t, pool))
	if !response.Allowed {
		t.Fatalf("expected request to be allowed, got denied: %s", response.Result.Message)
	}
}

func TestDNSUpstreamPoolUniquenessWebhookRejectsSecondPool(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	existing := &v1alpha1.DNSUpstreamPool{
		TypeMeta: metav1.TypeMeta{APIVersion: "dns.astradns.com/v1alpha1", Kind: "DNSUpstreamPool"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pool-a",
			Namespace: "test-ns",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	webhook := &DNSUpstreamPoolUniquenessWebhook{Client: client}

	pool := &v1alpha1.DNSUpstreamPool{
		TypeMeta: metav1.TypeMeta{APIVersion: "dns.astradns.com/v1alpha1", Kind: "DNSUpstreamPool"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pool-b",
			Namespace: "test-ns",
		},
	}

	response := webhook.Handle(context.Background(), newCreateRequest(t, pool))
	if response.Allowed {
		t.Fatalf("expected request to be denied")
	}
	if !strings.Contains(response.Result.Message, "only one DNSUpstreamPool is allowed per namespace") {
		t.Fatalf("unexpected deny message: %s", response.Result.Message)
	}
}

// --- Gap 13: Webhook edge cases ---

func TestDNSUpstreamPoolUniquenessWebhookAllowsUpdateOperation(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	existing := &v1alpha1.DNSUpstreamPool{
		TypeMeta: metav1.TypeMeta{APIVersion: "dns.astradns.com/v1alpha1", Kind: "DNSUpstreamPool"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pool-a",
			Namespace: "test-ns",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	webhook := &DNSUpstreamPoolUniquenessWebhook{Client: client}

	pool := &v1alpha1.DNSUpstreamPool{
		TypeMeta: metav1.TypeMeta{APIVersion: "dns.astradns.com/v1alpha1", Kind: "DNSUpstreamPool"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pool-a",
			Namespace: "test-ns",
		},
	}

	req := newAdmissionRequest(t, pool, admissionv1.Update)
	response := webhook.Handle(context.Background(), req)
	if !response.Allowed {
		t.Fatalf("expected Update operation to be allowed, got denied: %s", response.Result.Message)
	}
}

func TestDNSUpstreamPoolUniquenessWebhookAllowsDeleteOperation(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	existing := &v1alpha1.DNSUpstreamPool{
		TypeMeta: metav1.TypeMeta{APIVersion: "dns.astradns.com/v1alpha1", Kind: "DNSUpstreamPool"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pool-a",
			Namespace: "test-ns",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	webhook := &DNSUpstreamPoolUniquenessWebhook{Client: client}

	pool := &v1alpha1.DNSUpstreamPool{
		TypeMeta: metav1.TypeMeta{APIVersion: "dns.astradns.com/v1alpha1", Kind: "DNSUpstreamPool"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pool-a",
			Namespace: "test-ns",
		},
	}

	req := newAdmissionRequest(t, pool, admissionv1.Delete)
	response := webhook.Handle(context.Background(), req)
	if !response.Allowed {
		t.Fatalf("expected Delete operation to be allowed, got denied: %s", response.Result.Message)
	}
}

func TestDNSUpstreamPoolUniquenessWebhookMissingNamespaceBothPoolAndRequest(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	webhook := &DNSUpstreamPoolUniquenessWebhook{Client: client}

	// Pool with no namespace and request with no namespace should produce an error.
	pool := &v1alpha1.DNSUpstreamPool{
		TypeMeta: metav1.TypeMeta{APIVersion: "dns.astradns.com/v1alpha1", Kind: "DNSUpstreamPool"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pool-no-ns",
			Namespace: "",
		},
	}

	raw, err := json.Marshal(pool)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UID:       types.UID("test-uid-no-ns"),
		Namespace: "", // Request also has no namespace
		Name:      pool.Name,
		Operation: admissionv1.Create,
		Object: runtime.RawExtension{
			Raw: raw,
		},
	}}

	response := webhook.Handle(context.Background(), req)
	if response.Allowed {
		t.Fatalf("expected request to be denied when namespace is missing from both pool and request")
	}
	if response.Result.Code != http.StatusBadRequest {
		t.Errorf("expected HTTP 400, got %d", response.Result.Code)
	}
	if !strings.Contains(response.Result.Message, "namespace is required") {
		t.Errorf("expected namespace error message, got: %s", response.Result.Message)
	}
}

func TestDNSUpstreamPoolUniquenessWebhookFallsBackToRequestNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	webhook := &DNSUpstreamPoolUniquenessWebhook{Client: client}

	// Pool object has no namespace, but the admission request does.
	pool := &v1alpha1.DNSUpstreamPool{
		TypeMeta: metav1.TypeMeta{APIVersion: "dns.astradns.com/v1alpha1", Kind: "DNSUpstreamPool"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pool-no-obj-ns",
			Namespace: "",
		},
	}

	raw, err := json.Marshal(pool)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	req := admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UID:       types.UID("test-uid-req-ns"),
		Namespace: "from-request",
		Name:      pool.Name,
		Operation: admissionv1.Create,
		Object: runtime.RawExtension{
			Raw: raw,
		},
	}}

	response := webhook.Handle(context.Background(), req)
	if !response.Allowed {
		t.Fatalf("expected request to be allowed (first pool in namespace via request fallback), got denied: %s", response.Result.Message)
	}
}

func TestDNSUpstreamPoolUniquenessWebhookNilClientReturnsError(t *testing.T) {
	webhook := &DNSUpstreamPoolUniquenessWebhook{Client: nil}

	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	pool := &v1alpha1.DNSUpstreamPool{
		TypeMeta: metav1.TypeMeta{APIVersion: "dns.astradns.com/v1alpha1", Kind: "DNSUpstreamPool"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pool-a",
			Namespace: "test-ns",
		},
	}

	response := webhook.Handle(context.Background(), newCreateRequest(t, pool))
	if response.Allowed {
		t.Fatalf("expected request to fail when client is nil")
	}
	if response.Result.Code != http.StatusInternalServerError {
		t.Errorf("expected HTTP 500, got %d", response.Result.Code)
	}
}

// --- Helpers ---

func newCreateRequest(t *testing.T, pool *v1alpha1.DNSUpstreamPool) admission.Request {
	t.Helper()
	return newAdmissionRequest(t, pool, admissionv1.Create)
}

func newAdmissionRequest(t *testing.T, pool *v1alpha1.DNSUpstreamPool, operation admissionv1.Operation) admission.Request {
	t.Helper()

	raw, err := json.Marshal(pool)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UID:       types.UID("test-uid"),
		Namespace: pool.Namespace,
		Name:      pool.Name,
		Operation: operation,
		Object: runtime.RawExtension{
			Raw: raw,
		},
	}}
}
