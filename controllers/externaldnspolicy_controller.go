package controllers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	v1alpha1 "github.com/astradns/astradns-types/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const externalPolicyValidatedCondition = "Validated"

// ExternalDNSPolicyReconciler reconciles ExternalDNSPolicy objects.
type ExternalDNSPolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=dns.astradns.com,resources=externaldnspolicies,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=dns.astradns.com,resources=externaldnspolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dns.astradns.com,resources=dnsupstreampools,verbs=get;list;watch
// +kubebuilder:rbac:groups=dns.astradns.com,resources=dnscacheprofiles,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile validates ExternalDNSPolicy references and updates status conditions.
func (r *ExternalDNSPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("externalDNSPolicy", req.NamespacedName)

	var policy v1alpha1.ExternalDNSPolicy
	if err := r.Get(ctx, req.NamespacedName, &policy); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get ExternalDNSPolicy %s: %w", req.NamespacedName, err)
	}

	if err := r.validateReferences(ctx, &policy); err != nil {
		logger.Error(err, "Invalid ExternalDNSPolicy references")
		r.recordEvent(&policy, corev1.EventTypeWarning, "ValidationFailed", err.Error())
		if statusErr := r.setValidatedCondition(
			ctx,
			&policy,
			metav1.ConditionFalse,
			"ValidationFailed",
			err.Error(),
		); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	r.recordEvent(&policy, corev1.EventTypeNormal, "Validated", "Validated ExternalDNSPolicy references")
	if err := r.setValidatedCondition(
		ctx,
		&policy,
		metav1.ConditionTrue,
		"Validated",
		"References are valid",
	); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ExternalDNSPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ExternalDNSPolicy{}).
		Watches(&v1alpha1.DNSUpstreamPool{}, handler.EnqueueRequestsFromMapFunc(r.mapUpstreamPoolToPolicies)).
		Watches(&v1alpha1.DNSCacheProfile{}, handler.EnqueueRequestsFromMapFunc(r.mapCacheProfileToPolicies)).
		Complete(r)
}

func (r *ExternalDNSPolicyReconciler) mapUpstreamPoolToPolicies(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	var policies v1alpha1.ExternalDNSPolicyList
	if err := r.List(ctx, &policies, client.InNamespace(obj.GetNamespace())); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, 0)
	for i := range policies.Items {
		policy := policies.Items[i]
		if strings.TrimSpace(policy.Spec.UpstreamPoolRef.Name) != obj.GetName() {
			continue
		}
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: policy.Namespace,
			Name:      policy.Name,
		}})
	}

	return requests
}

func (r *ExternalDNSPolicyReconciler) mapCacheProfileToPolicies(
	ctx context.Context,
	obj client.Object,
) []reconcile.Request {
	var policies v1alpha1.ExternalDNSPolicyList
	if err := r.List(ctx, &policies, client.InNamespace(obj.GetNamespace())); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, 0)
	for i := range policies.Items {
		policy := policies.Items[i]
		if strings.TrimSpace(policy.Spec.CacheProfileRef.Name) != obj.GetName() {
			continue
		}
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: policy.Namespace,
			Name:      policy.Name,
		}})
	}

	return requests
}

func (r *ExternalDNSPolicyReconciler) validateReferences(
	ctx context.Context,
	policy *v1alpha1.ExternalDNSPolicy,
) error {
	if policy == nil {
		return errors.New("external dns policy is nil")
	}

	if len(policy.Spec.Selector.Namespaces) == 0 {
		return errors.New("spec.selector.namespaces must contain at least one namespace")
	}
	seenSelectorNamespaces := make(map[string]struct{}, len(policy.Spec.Selector.Namespaces))
	for i, namespace := range policy.Spec.Selector.Namespaces {
		trimmed := strings.TrimSpace(namespace)
		if trimmed == "" {
			return fmt.Errorf("spec.selector.namespaces[%d] must not be empty", i)
		}
		if errs := validation.IsDNS1123Label(trimmed); len(errs) > 0 {
			return fmt.Errorf("spec.selector.namespaces[%d] %q is not a valid namespace name", i, namespace)
		}
		if _, exists := seenSelectorNamespaces[trimmed]; exists {
			return fmt.Errorf("spec.selector.namespaces[%d] %q is duplicated", i, namespace)
		}
		seenSelectorNamespaces[trimmed] = struct{}{}
	}

	upstreamPoolName, err := validatePolicyRefName(policy.Spec.UpstreamPoolRef.Name, "spec.upstreamPoolRef.name", true)
	if err != nil {
		return err
	}

	if err := r.Get(
		ctx,
		types.NamespacedName{Namespace: policy.Namespace, Name: upstreamPoolName},
		&v1alpha1.DNSUpstreamPool{},
	); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("referenced upstream pool %q does not exist", upstreamPoolName)
		}
		return fmt.Errorf("get referenced upstream pool %q: %w", upstreamPoolName, err)
	}

	cacheProfileName, err := validatePolicyRefName(policy.Spec.CacheProfileRef.Name, "spec.cacheProfileRef.name", false)
	if err != nil {
		return err
	}
	if cacheProfileName == "" {
		return nil
	}

	if err := r.Get(
		ctx,
		types.NamespacedName{Namespace: policy.Namespace, Name: cacheProfileName},
		&v1alpha1.DNSCacheProfile{},
	); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("referenced cache profile %q does not exist", cacheProfileName)
		}
		return fmt.Errorf("get referenced cache profile %q: %w", cacheProfileName, err)
	}

	return nil
}

func validatePolicyRefName(raw, fieldPath string, required bool) (string, error) {
	trimmed := strings.TrimSpace(raw)

	if trimmed == "" {
		if required {
			return "", fmt.Errorf("%s is required", fieldPath)
		}
		if raw != "" {
			return "", fmt.Errorf("%s must not be whitespace", fieldPath)
		}
		return "", nil
	}

	if raw != trimmed {
		return "", fmt.Errorf("%s must not include leading or trailing whitespace", fieldPath)
	}

	if errs := validation.IsDNS1123Subdomain(trimmed); len(errs) > 0 {
		return "", fmt.Errorf("%s %q is not a valid resource name", fieldPath, raw)
	}

	return trimmed, nil
}

func (r *ExternalDNSPolicyReconciler) setValidatedCondition(
	ctx context.Context,
	policy *v1alpha1.ExternalDNSPolicy,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	base := policy.DeepCopy()
	meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
		Type:               externalPolicyValidatedCondition,
		Status:             status,
		ObservedGeneration: policy.Generation,
		Reason:             reason,
		Message:            message,
	})
	policy.Status.ObservedGeneration = policy.Generation

	if err := r.Status().Patch(ctx, policy, client.MergeFrom(base)); err != nil {
		return fmt.Errorf("update ExternalDNSPolicy status: %w", err)
	}

	return nil
}

func (r *ExternalDNSPolicyReconciler) recordEvent(
	object *v1alpha1.ExternalDNSPolicy,
	eventType,
	reason,
	message string,
) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(object, nil, eventType, reason, "Reconcile", "%s", message)
}

var _ reconcile.Reconciler = (*ExternalDNSPolicyReconciler)(nil)
