package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	v1alpha1 "github.com/astradns/astradns-types/api/v1alpha1"
	admissionv1 "k8s.io/api/admission/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	poolUniquenessWebhookPath = "/validate-dns-astradns-com-v1alpha1-dnsupstreampool"
)

// DNSUpstreamPoolUniquenessWebhook rejects creating a second DNSUpstreamPool in the same namespace.
type DNSUpstreamPoolUniquenessWebhook struct {
	Client client.Client
}

// SetupWithManager registers the validating webhook endpoint.
func (w *DNSUpstreamPoolUniquenessWebhook) SetupWithManager(mgr ctrl.Manager) error {
	if w.Client == nil {
		w.Client = mgr.GetClient()
	}

	mgr.GetWebhookServer().Register(poolUniquenessWebhookPath, &admission.Webhook{Handler: w})
	return nil
}

// Handle validates DNSUpstreamPool creation requests.
func (w *DNSUpstreamPoolUniquenessWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Create {
		return admission.Allowed("operation does not create a new DNSUpstreamPool")
	}
	if w.Client == nil {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("webhook client is not configured"))
	}

	pool := &v1alpha1.DNSUpstreamPool{}
	if err := json.Unmarshal(req.Object.Raw, pool); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decode DNSUpstreamPool: %w", err))
	}

	namespace := strings.TrimSpace(pool.Namespace)
	if namespace == "" {
		namespace = strings.TrimSpace(req.Namespace)
	}
	if namespace == "" {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("DNSUpstreamPool namespace is required"))
	}

	var pools v1alpha1.DNSUpstreamPoolList
	if err := w.Client.List(ctx, &pools, client.InNamespace(namespace)); err != nil {
		return admission.Errored(
			http.StatusInternalServerError,
			fmt.Errorf("list DNSUpstreamPools in namespace %q: %w", namespace, err),
		)
	}

	if len(pools.Items) == 0 {
		return admission.Allowed("first DNSUpstreamPool in namespace")
	}

	names := make([]string, 0, len(pools.Items))
	for _, existing := range pools.Items {
		names = append(names, existing.Name)
	}
	sort.Strings(names)

	message := fmt.Sprintf(
		"only one DNSUpstreamPool is allowed per namespace; existing pools: %s",
		strings.Join(names, ", "),
	)
	return admission.Denied(message)
}

var _ admission.Handler = (*DNSUpstreamPoolUniquenessWebhook)(nil)
