package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	v1alpha1 "github.com/astradns/astradns-types/api/v1alpha1"
	typesengineconfig "github.com/astradns/astradns-types/engineconfig"
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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	agentConfigMapName      = "astradns-agent-config"
	agentConfigMapNameEnv   = "ASTRADNS_AGENT_CONFIGMAP_NAME"
	agentConfigKey          = "config.json"
	defaultCacheProfileName = "default"
	supersededPoolReason    = "Superseded"

	upstreamPoolReadyCondition = "Ready"

	// ConfigMap objects are capped at 1MiB. Keep a small safety headroom for
	// metadata and key overhead so writes fail early with a clear status message.
	configMapObjectSizeLimitBytes = 1 << 20
	configMapSafetyOverheadBytes  = 4 << 10
	maxAgentConfigJSONBytes       = configMapObjectSizeLimitBytes - configMapSafetyOverheadBytes

	configMapUpdateFailureThreshold = 3
	configMapCircuitOpenInterval    = 30 * time.Second

	// initialRVAnnotation stores the ResourceVersion observed when the controller
	// first reconciles a pool. Unlike the live ResourceVersion (which changes on
	// every update), this value is monotonically ordered in etcd and reflects true
	// creation order — even when two pools share the same second-precision
	// creationTimestamp.
	initialRVAnnotation = "dns.astradns.com/initial-resource-version"
)

var errConfigMapCircuitOpen = errors.New("configmap update circuit breaker is open")

// DNSUpstreamPoolReconciler reconciles DNSUpstreamPool objects.
type DNSUpstreamPoolReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	ConfigGen      typesengineconfig.ConfigGenerator
	ConfigRenderer typesengineconfig.ConfigRenderer
	Recorder       events.EventRecorder

	configMapCircuitMu        sync.Mutex
	configMapFailureCounts    map[string]int
	configMapCircuitOpenUntil map[string]time.Time
}

// +kubebuilder:rbac:groups=dns.astradns.com,resources=dnsupstreampools,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=dns.astradns.com,resources=dnsupstreampools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dns.astradns.com,resources=dnscacheprofiles,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile reconciles DNSUpstreamPool resources and writes rendered engine config to a ConfigMap.
func (r *DNSUpstreamPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if r.ConfigGen == nil || r.ConfigRenderer == nil {
		return ctrl.Result{}, errors.New("config generator and renderer must be configured")
	}

	logger := log.FromContext(ctx).WithValues("dnsUpstreamPool", req.NamespacedName)

	var pool v1alpha1.DNSUpstreamPool
	if err := r.Get(ctx, req.NamespacedName, &pool); err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.reconcileAfterPoolDeletion(ctx, req.Namespace); err != nil {
				return ctrl.Result{}, fmt.Errorf("regenerate config after delete: %w", err)
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("get DNSUpstreamPool %s: %w", req.NamespacedName, err)
	}

	// Stamp the initial ResourceVersion so that pool selection can use a stable,
	// creation-ordered tiebreaker instead of the live (mutable) ResourceVersion.
	if err := r.ensureInitialRVAnnotation(ctx, &pool); err != nil {
		return ctrl.Result{}, fmt.Errorf("stamp initial resource version: %w", err)
	}

	if err := validateDNSUpstreamPool(&pool); err != nil {
		logger.Error(err, "Invalid DNSUpstreamPool spec")
		r.recordEvent(&pool, corev1.EventTypeWarning, "InvalidSpec", err.Error())
		if statusErr := r.setReadyCondition(
			ctx,
			&pool,
			metav1.ConditionFalse,
			"InvalidSpec",
			err.Error(),
		); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	activePoolName, poolCount, err := r.selectActivePoolName(ctx, pool.Namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	if poolCount > 1 {
		if pool.Name != activePoolName {
			message := fmt.Sprintf("pool %q is active in this namespace", activePoolName)
			r.recordEvent(&pool, corev1.EventTypeWarning, supersededPoolReason, message)
			if statusErr := r.setReadyCondition(
				ctx,
				&pool,
				metav1.ConditionFalse,
				supersededPoolReason,
				message,
			); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			return ctrl.Result{}, nil
		}

		logger.Info(
			"multiple DNSUpstreamPools detected, reconciling active pool",
			"active_pool",
			activePoolName,
			"count",
			poolCount,
		)
	}

	profile, err := r.getDefaultCacheProfile(ctx, pool.Namespace)
	if err != nil {
		err = fmt.Errorf("get default DNSCacheProfile: %w", err)
		logger.Error(err, "Could not fetch default cache profile")
		r.recordEvent(&pool, corev1.EventTypeWarning, "ProfileLookupFailed", err.Error())
		if statusErr := r.setReadyCondition(
			ctx,
			&pool,
			metav1.ConditionFalse,
			"ProfileLookupFailed",
			err.Error(),
		); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, err
	}

	config, err := r.ConfigGen.Generate(&pool, profile)
	if err != nil {
		err = fmt.Errorf("generate engine config: %w", err)
		logger.Error(err, "Could not generate engine configuration")
		r.recordEvent(&pool, corev1.EventTypeWarning, "ConfigGenerationFailed", err.Error())
		if statusErr := r.setReadyCondition(
			ctx,
			&pool,
			metav1.ConditionFalse,
			"ConfigGenerationFailed",
			err.Error(),
		); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	// Validate config can be rendered (catches template errors early)
	if _, err := r.ConfigRenderer.Render(config); err != nil {
		err = fmt.Errorf("validate engine config: %w", err)
		logger.Error(err, "Config validation failed")
		r.recordEvent(&pool, corev1.EventTypeWarning, "ValidationFailed", err.Error())
		if statusErr := r.setReadyCondition(
			ctx,
			&pool,
			metav1.ConditionFalse,
			"ValidationFailed",
			err.Error(),
		); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	// Marshal EngineConfig as JSON for the agent
	configJSON, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		err = fmt.Errorf("marshal engine config: %w", err)
		logger.Error(err, "Could not marshal engine configuration to JSON")
		r.recordEvent(&pool, corev1.EventTypeWarning, "MarshalFailed", err.Error())
		if statusErr := r.setReadyCondition(
			ctx,
			&pool,
			metav1.ConditionFalse,
			"MarshalFailed",
			err.Error(),
		); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	if err := validateConfigMapPayloadSize(string(configJSON)); err != nil {
		logger.Error(err, "Rendered config exceeded ConfigMap payload limit")
		r.recordEvent(&pool, corev1.EventTypeWarning, "ConfigTooLarge", err.Error())
		if statusErr := r.setReadyCondition(
			ctx,
			&pool,
			metav1.ConditionFalse,
			"ConfigTooLarge",
			err.Error(),
		); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	operatorNamespace := r.operatorNamespace(pool.Namespace)
	if err := r.upsertConfigMapWithCircuitBreaker(ctx, operatorNamespace, string(configJSON)); err != nil {
		err = fmt.Errorf("upsert configmap: %w", err)
		reason := "ConfigMapUpdateFailed"
		result := ctrl.Result{}
		if errors.Is(err, errConfigMapCircuitOpen) {
			reason = "ConfigMapCircuitOpen"
			result.RequeueAfter = configMapCircuitOpenInterval
		}
		logger.Error(err, "Could not update config map")
		r.recordEvent(&pool, corev1.EventTypeWarning, reason, err.Error())
		if statusErr := r.setReadyCondition(
			ctx,
			&pool,
			metav1.ConditionFalse,
			reason,
			err.Error(),
		); statusErr != nil {
			return result, statusErr
		}
		if errors.Is(err, errConfigMapCircuitOpen) {
			return result, nil
		}
		return result, err
	}

	r.recordEvent(&pool, corev1.EventTypeNormal, "ConfigRendered", "Rendered engine configuration")
	if err := r.setReadyCondition(
		ctx,
		&pool,
		metav1.ConditionTrue,
		"Ready",
		"Configuration rendered successfully",
	); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DNSUpstreamPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.DNSUpstreamPool{}).
		Watches(&v1alpha1.DNSCacheProfile{}, handler.EnqueueRequestsFromMapFunc(r.mapCacheProfileToPools)).
		Complete(r)
}

func (r *DNSUpstreamPoolReconciler) mapCacheProfileToPools(ctx context.Context, obj client.Object) []reconcile.Request {
	var pools v1alpha1.DNSUpstreamPoolList
	if err := r.List(ctx, &pools, client.InNamespace(obj.GetNamespace())); err != nil {
		return nil
	}

	requests := make([]reconcile.Request, 0, len(pools.Items))
	for i := range pools.Items {
		pool := pools.Items[i]
		requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: pool.Namespace,
			Name:      pool.Name,
		}})
	}

	return requests
}

func (r *DNSUpstreamPoolReconciler) reconcileAfterPoolDeletion(ctx context.Context, poolNamespace string) error {
	logger := log.FromContext(ctx)

	var pools v1alpha1.DNSUpstreamPoolList
	if err := r.List(ctx, &pools, client.InNamespace(poolNamespace)); err != nil {
		return fmt.Errorf("list remaining DNSUpstreamPools: %w", err)
	}

	configNamespace := r.operatorNamespace(poolNamespace)
	if len(pools.Items) == 0 {
		return r.removeConfigKey(ctx, configNamespace)
	}

	sortPoolsForSelection(pools.Items)

	if len(pools.Items) > 1 {
		logger.Info("multiple DNSUpstreamPools in namespace, using oldest pool",
			"namespace", poolNamespace,
			"selected", pools.Items[0].Name,
			"count", len(pools.Items),
		)
	}

	profile, err := r.getDefaultCacheProfile(ctx, poolNamespace)
	if err != nil {
		return fmt.Errorf("get default DNSCacheProfile: %w", err)
	}

	config, err := r.ConfigGen.Generate(&pools.Items[0], profile)
	if err != nil {
		return fmt.Errorf("generate engine config from remaining pool: %w", err)
	}

	// Validate config can be rendered (catches template errors early)
	if _, err := r.ConfigRenderer.Render(config); err != nil {
		return fmt.Errorf("validate config from remaining pool: %w", err)
	}

	configJSON, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config from remaining pool: %w", err)
	}

	if err := validateConfigMapPayloadSize(string(configJSON)); err != nil {
		r.recordEvent(&pools.Items[0], corev1.EventTypeWarning, "ConfigTooLarge", err.Error())
		if statusErr := r.setReadyCondition(
			ctx,
			&pools.Items[0],
			metav1.ConditionFalse,
			"ConfigTooLarge",
			err.Error(),
		); statusErr != nil {
			return fmt.Errorf("update oversized pool status: %w", statusErr)
		}
		return nil
	}

	if err := r.upsertConfigMapWithCircuitBreaker(ctx, configNamespace, string(configJSON)); err != nil {
		if errors.Is(err, errConfigMapCircuitOpen) {
			r.recordEvent(&pools.Items[0], corev1.EventTypeWarning, "ConfigMapCircuitOpen", err.Error())
			if statusErr := r.setReadyCondition(
				ctx,
				&pools.Items[0],
				metav1.ConditionFalse,
				"ConfigMapCircuitOpen",
				err.Error(),
			); statusErr != nil {
				return fmt.Errorf("update circuit-open pool status: %w", statusErr)
			}
			return nil
		}
		return err
	}

	if err := r.setReadyCondition(
		ctx,
		&pools.Items[0],
		metav1.ConditionTrue,
		"Ready",
		"Configuration rendered successfully",
	); err != nil {
		return fmt.Errorf("update remaining pool status: %w", err)
	}

	return nil
}

func validateConfigMapPayloadSize(renderedConfig string) error {
	payloadSize := len(renderedConfig) + len(agentConfigKey)
	if payloadSize > maxAgentConfigJSONBytes {
		return fmt.Errorf(
			"rendered config payload is %d bytes, exceeds safe ConfigMap limit of %d bytes",
			payloadSize,
			maxAgentConfigJSONBytes,
		)
	}

	return nil
}

func (r *DNSUpstreamPoolReconciler) selectActivePoolName(ctx context.Context, namespace string) (string, int, error) {
	var pools v1alpha1.DNSUpstreamPoolList
	if err := r.List(ctx, &pools, client.InNamespace(namespace)); err != nil {
		return "", 0, fmt.Errorf("list DNSUpstreamPools in namespace %q: %w", namespace, err)
	}

	if len(pools.Items) == 0 {
		return "", 0, nil
	}

	sortPoolsForSelection(pools.Items)

	return pools.Items[0].Name, len(pools.Items), nil
}

func sortPoolsForSelection(items []v1alpha1.DNSUpstreamPool) {
	sort.Slice(items, func(i, j int) bool {
		// 1. Oldest creationTimestamp wins.
		left := items[i].CreationTimestamp.Time
		right := items[j].CreationTimestamp.Time

		if !left.IsZero() && !right.IsZero() && !left.Equal(right) {
			return left.Before(right)
		}

		// 2. Stable initial ResourceVersion (stamped once at first reconcile).
		//    Unlike the live RV, this never changes after creation so it
		//    reflects true etcd insertion order.
		leftIRV := initialRV(items[i])
		rightIRV := initialRV(items[j])
		if leftIRV > 0 && rightIRV > 0 && leftIRV != rightIRV {
			return leftIRV < rightIRV
		}

		// 3. Alphabetical name as ultimate deterministic tiebreaker.
		return items[i].Name < items[j].Name
	})
}

// initialRV returns the stamped initial ResourceVersion or 0 if absent/unparseable.
func initialRV(pool v1alpha1.DNSUpstreamPool) int64 {
	if pool.Annotations == nil {
		return 0
	}
	v, err := strconv.ParseInt(pool.Annotations[initialRVAnnotation], 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// ensureInitialRVAnnotation stamps the pool with its ResourceVersion at first
// reconcile so that sortPoolsForSelection can use a stable, creation-ordered
// tiebreaker. The annotation is written once and never updated.
func (r *DNSUpstreamPoolReconciler) ensureInitialRVAnnotation(
	ctx context.Context,
	pool *v1alpha1.DNSUpstreamPool,
) error {
	if pool.Annotations != nil {
		if _, ok := pool.Annotations[initialRVAnnotation]; ok {
			return nil // already stamped
		}
	}

	base := pool.DeepCopy()
	if pool.Annotations == nil {
		pool.Annotations = map[string]string{}
	}
	pool.Annotations[initialRVAnnotation] = pool.ResourceVersion

	if err := r.Patch(ctx, pool, client.MergeFrom(base)); err != nil {
		return fmt.Errorf("patch initial-resource-version annotation: %w", err)
	}

	return nil
}

func (r *DNSUpstreamPoolReconciler) upsertConfigMap(ctx context.Context, namespace, renderedConfig string) error {
	configMapName := r.resolvedAgentConfigMapName()

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, configMap, func() error {
		if configMap.Data == nil {
			configMap.Data = map[string]string{}
		}
		configMap.Data[agentConfigKey] = renderedConfig
		return nil
	})
	if err != nil {
		return fmt.Errorf("create or update ConfigMap %s/%s: %w", namespace, configMapName, err)
	}

	return nil
}

func (r *DNSUpstreamPoolReconciler) upsertConfigMapWithCircuitBreaker(
	ctx context.Context,
	namespace,
	renderedConfig string,
) error {
	if openUntil, open := r.isConfigMapCircuitOpen(namespace); open {
		return fmt.Errorf(
			"%w: namespace %q until %s",
			errConfigMapCircuitOpen,
			namespace,
			openUntil.UTC().Format(time.RFC3339),
		)
	}

	if err := r.upsertConfigMap(ctx, namespace, renderedConfig); err != nil {
		r.recordConfigMapUpdateFailure(namespace)
		return err
	}

	r.resetConfigMapUpdateFailures(namespace)
	return nil
}

func (r *DNSUpstreamPoolReconciler) recordConfigMapUpdateFailure(namespace string) {
	r.configMapCircuitMu.Lock()
	defer r.configMapCircuitMu.Unlock()

	r.ensureConfigMapCircuitStateLocked()

	if openUntil, open := r.configMapCircuitOpenUntil[namespace]; open {
		if time.Now().Before(openUntil) {
			return
		}
		delete(r.configMapCircuitOpenUntil, namespace)
	}

	nextFailureCount := r.configMapFailureCounts[namespace] + 1
	if nextFailureCount >= configMapUpdateFailureThreshold {
		r.configMapCircuitOpenUntil[namespace] = time.Now().Add(configMapCircuitOpenInterval)
		r.configMapFailureCounts[namespace] = 0
		return
	}

	r.configMapFailureCounts[namespace] = nextFailureCount
}

func (r *DNSUpstreamPoolReconciler) resetConfigMapUpdateFailures(namespace string) {
	r.configMapCircuitMu.Lock()
	defer r.configMapCircuitMu.Unlock()

	r.ensureConfigMapCircuitStateLocked()
	delete(r.configMapFailureCounts, namespace)
	delete(r.configMapCircuitOpenUntil, namespace)
}

func (r *DNSUpstreamPoolReconciler) isConfigMapCircuitOpen(namespace string) (time.Time, bool) {
	r.configMapCircuitMu.Lock()
	defer r.configMapCircuitMu.Unlock()

	r.ensureConfigMapCircuitStateLocked()

	openUntil, open := r.configMapCircuitOpenUntil[namespace]
	if !open {
		return time.Time{}, false
	}

	if time.Now().After(openUntil) {
		delete(r.configMapCircuitOpenUntil, namespace)
		return time.Time{}, false
	}

	return openUntil, true
}

func (r *DNSUpstreamPoolReconciler) ensureConfigMapCircuitStateLocked() {
	if r.configMapFailureCounts == nil {
		r.configMapFailureCounts = make(map[string]int)
	}
	if r.configMapCircuitOpenUntil == nil {
		r.configMapCircuitOpenUntil = make(map[string]time.Time)
	}
}

func (r *DNSUpstreamPoolReconciler) removeConfigKey(ctx context.Context, namespace string) error {
	configMapName := r.resolvedAgentConfigMapName()

	configMap := &corev1.ConfigMap{}
	namespacedName := types.NamespacedName{Name: configMapName, Namespace: namespace}
	if err := r.Get(ctx, namespacedName, configMap); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get ConfigMap %s/%s: %w", namespace, configMapName, err)
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, configMap, func() error {
		if configMap.Data == nil {
			configMap.Data = map[string]string{}
			return nil
		}
		delete(configMap.Data, agentConfigKey)
		return nil
	})
	if err != nil {
		return fmt.Errorf("remove config key from ConfigMap %s/%s: %w", namespace, configMapName, err)
	}

	return nil
}

func (r *DNSUpstreamPoolReconciler) getDefaultCacheProfile(
	ctx context.Context,
	namespace string,
) (*v1alpha1.DNSCacheProfile, error) {
	profile := &v1alpha1.DNSCacheProfile{}
	err := r.Get(ctx, types.NamespacedName{Name: defaultCacheProfileName, Namespace: namespace}, profile)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return profile, nil
}

func (r *DNSUpstreamPoolReconciler) setReadyCondition(
	ctx context.Context,
	pool *v1alpha1.DNSUpstreamPool,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	base := pool.DeepCopy()
	meta.SetStatusCondition(&pool.Status.Conditions, metav1.Condition{
		Type:               upstreamPoolReadyCondition,
		Status:             status,
		ObservedGeneration: pool.Generation,
		Reason:             reason,
		Message:            message,
	})
	pool.Status.ObservedGeneration = pool.Generation

	if err := r.Status().Patch(ctx, pool, client.MergeFrom(base)); err != nil {
		return fmt.Errorf("update DNSUpstreamPool status: %w", err)
	}

	return nil
}

func validateDNSUpstreamPool(pool *v1alpha1.DNSUpstreamPool) error {
	if pool == nil {
		return errors.New("dns upstream pool is nil")
	}

	if len(pool.Spec.Upstreams) == 0 {
		return errors.New("spec.upstreams must contain at least one upstream")
	}

	seenUpstreams := make(map[string]struct{}, len(pool.Spec.Upstreams))

	for i, upstream := range pool.Spec.Upstreams {
		trimmedAddress := strings.TrimSpace(upstream.Address)
		if upstream.Address != trimmedAddress {
			return fmt.Errorf("spec.upstreams[%d].address must not include leading or trailing whitespace", i)
		}

		if !isValidUpstreamAddress(trimmedAddress) {
			return fmt.Errorf("spec.upstreams[%d].address %q is not a valid IP or DNS name", i, upstream.Address)
		}
		if upstream.Port <= 0 || upstream.Port > 65535 {
			return fmt.Errorf("spec.upstreams[%d].port must be between 1 and 65535", i)
		}

		upstreamKey := fmt.Sprintf("%s:%d", trimmedAddress, upstream.Port)
		if _, exists := seenUpstreams[upstreamKey]; exists {
			return fmt.Errorf("spec.upstreams[%d] %q is duplicated", i, upstreamKey)
		}
		seenUpstreams[upstreamKey] = struct{}{}
	}

	return nil
}

func isValidUpstreamAddress(address string) bool {
	trimmed := strings.TrimSpace(address)
	if trimmed == "" {
		return false
	}

	if _, err := netip.ParseAddr(trimmed); err == nil {
		return true
	}
	if looksLikeInvalidIPv4Literal(trimmed) {
		return false
	}

	return len(validation.IsDNS1123Subdomain(trimmed)) == 0
}

func looksLikeInvalidIPv4Literal(value string) bool {
	if strings.Count(value, ".") != 3 {
		return false
	}
	for _, r := range value {
		if r != '.' && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func (r *DNSUpstreamPoolReconciler) operatorNamespace(fallback string) string {
	if namespace := strings.TrimSpace(os.Getenv("POD_NAMESPACE")); namespace != "" {
		return namespace
	}
	return fallback
}

func (r *DNSUpstreamPoolReconciler) resolvedAgentConfigMapName() string {
	if name := strings.TrimSpace(os.Getenv(agentConfigMapNameEnv)); name != "" {
		return name
	}
	return agentConfigMapName
}

func (r *DNSUpstreamPoolReconciler) recordEvent(object *v1alpha1.DNSUpstreamPool, eventType, reason, message string) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(object, nil, eventType, reason, "Reconcile", "%s", message)
}

var _ reconcile.Reconciler = (*DNSUpstreamPoolReconciler)(nil)
