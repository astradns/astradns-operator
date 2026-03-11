package controllers

import (
	"context"
	"encoding/json"
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

func newCreateRequest(t *testing.T, pool *v1alpha1.DNSUpstreamPool) admission.Request {
	t.Helper()

	raw, err := json.Marshal(pool)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		UID:       types.UID("test-uid"),
		Namespace: pool.Namespace,
		Name:      pool.Name,
		Operation: admissionv1.Create,
		Object: runtime.RawExtension{
			Raw: raw,
		},
	}}
}
