package controllers

import (
	"context"
	"errors"
	"fmt"

	v1alpha1 "github.com/astradns/astradns-types/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	cacheProfileActiveCondition = "Active"

	defaultProfilePositiveTTLMin = 60
	defaultProfilePositiveTTLMax = 300
)

// DNSCacheProfileReconciler reconciles DNSCacheProfile objects.
type DNSCacheProfileReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

// +kubebuilder:rbac:groups=dns.astradns.com,resources=dnscacheprofiles,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=dns.astradns.com,resources=dnscacheprofiles/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile validates DNSCacheProfile and updates status conditions.
func (r *DNSCacheProfileReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("dnsCacheProfile", req.NamespacedName)

	var profile v1alpha1.DNSCacheProfile
	if err := r.Get(ctx, req.NamespacedName, &profile); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get DNSCacheProfile %s: %w", req.NamespacedName, err)
	}

	if err := validateDNSCacheProfile(&profile); err != nil {
		logger.Error(err, "Invalid DNSCacheProfile spec")
		r.recordEvent(&profile, corev1.EventTypeWarning, "InvalidSpec", err.Error())
		if statusErr := r.setActiveCondition(
			ctx,
			&profile,
			metav1.ConditionFalse,
			"InvalidSpec",
			err.Error(),
		); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	r.recordEvent(&profile, corev1.EventTypeNormal, "Validated", "Validated DNSCacheProfile")
	if err := r.setActiveCondition(
		ctx,
		&profile,
		metav1.ConditionTrue,
		"Active",
		"DNSCacheProfile is active",
	); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DNSCacheProfileReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.DNSCacheProfile{}).
		Complete(r)
}

func (r *DNSCacheProfileReconciler) setActiveCondition(
	ctx context.Context,
	profile *v1alpha1.DNSCacheProfile,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	base := profile.DeepCopy()
	meta.SetStatusCondition(&profile.Status.Conditions, metav1.Condition{
		Type:               cacheProfileActiveCondition,
		Status:             status,
		ObservedGeneration: profile.Generation,
		Reason:             reason,
		Message:            message,
	})

	if err := r.Status().Patch(ctx, profile, client.MergeFrom(base)); err != nil {
		return fmt.Errorf("update DNSCacheProfile status: %w", err)
	}

	return nil
}

func validateDNSCacheProfile(profile *v1alpha1.DNSCacheProfile) error {
	if profile == nil {
		return errors.New("dns cache profile is nil")
	}

	if profile.Spec.MaxEntries < 0 {
		return errors.New("spec.maxEntries must be >= 0")
	}
	if profile.Spec.PositiveTtl.MinSeconds < 0 {
		return errors.New("spec.positiveTtl.minSeconds must be >= 0")
	}
	if profile.Spec.PositiveTtl.MaxSeconds < 0 {
		return errors.New("spec.positiveTtl.maxSeconds must be >= 0")
	}
	if profile.Spec.NegativeTtl.Seconds < 0 {
		return errors.New("spec.negativeTtl.seconds must be >= 0")
	}
	if profile.Spec.Prefetch.Threshold < 0 {
		return errors.New("spec.prefetch.threshold must be >= 0")
	}

	minTTL := int(profile.Spec.PositiveTtl.MinSeconds)
	if minTTL == 0 {
		minTTL = defaultProfilePositiveTTLMin
	}
	maxTTL := int(profile.Spec.PositiveTtl.MaxSeconds)
	if maxTTL == 0 {
		maxTTL = defaultProfilePositiveTTLMax
	}

	if minTTL > maxTTL {
		return fmt.Errorf("spec.positiveTtl.minSeconds (%d) must be <= spec.positiveTtl.maxSeconds (%d)", minTTL, maxTTL)
	}

	return nil
}

func (r *DNSCacheProfileReconciler) recordEvent(object *v1alpha1.DNSCacheProfile, eventType, reason, message string) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(object, nil, eventType, reason, "Reconcile", "%s", message)
}

var _ reconcile.Reconciler = (*DNSCacheProfileReconciler)(nil)
