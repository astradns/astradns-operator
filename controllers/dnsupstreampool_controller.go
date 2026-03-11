package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"sort"
	"strings"

	v1alpha1 "github.com/astradns/astradns-types/api/v1alpha1"
	typesengineconfig "github.com/astradns/astradns-types/engineconfig"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/tools/record"
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

	upstreamPoolReadyCondition = "Ready"
)

// DNSUpstreamPoolReconciler reconciles DNSUpstreamPool objects.
type DNSUpstreamPoolReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	ConfigGen      typesengineconfig.ConfigGenerator
	ConfigRenderer typesengineconfig.ConfigRenderer
	Recorder       record.EventRecorder
}

// +kubebuilder:rbac:groups=dns.astradns.com,resources=dnsupstreampools,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=dns.astradns.com,resources=dnsupstreampools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dns.astradns.com,resources=dnscacheprofiles,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch
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

	if err := validateDNSUpstreamPool(&pool); err != nil {
		logger.Error(err, "Invalid DNSUpstreamPool spec")
		r.recordEvent(&pool, corev1.EventTypeWarning, "InvalidSpec", err.Error())
		if statusErr := r.setReadyCondition(ctx, &pool, metav1.ConditionFalse, "InvalidSpec", err.Error()); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	profile, err := r.getDefaultCacheProfile(ctx, pool.Namespace)
	if err != nil {
		err = fmt.Errorf("get default DNSCacheProfile: %w", err)
		logger.Error(err, "Could not fetch default cache profile")
		r.recordEvent(&pool, corev1.EventTypeWarning, "ProfileLookupFailed", err.Error())
		if statusErr := r.setReadyCondition(ctx, &pool, metav1.ConditionFalse, "ProfileLookupFailed", err.Error()); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, err
	}

	config, err := r.ConfigGen.Generate(&pool, profile)
	if err != nil {
		err = fmt.Errorf("generate engine config: %w", err)
		logger.Error(err, "Could not generate engine configuration")
		r.recordEvent(&pool, corev1.EventTypeWarning, "ConfigGenerationFailed", err.Error())
		if statusErr := r.setReadyCondition(ctx, &pool, metav1.ConditionFalse, "ConfigGenerationFailed", err.Error()); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	// Validate config can be rendered (catches template errors early)
	if _, err := r.ConfigRenderer.Render(config); err != nil {
		err = fmt.Errorf("validate engine config: %w", err)
		logger.Error(err, "Config validation failed")
		r.recordEvent(&pool, corev1.EventTypeWarning, "ValidationFailed", err.Error())
		if statusErr := r.setReadyCondition(ctx, &pool, metav1.ConditionFalse, "ValidationFailed", err.Error()); statusErr != nil {
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
		if statusErr := r.setReadyCondition(ctx, &pool, metav1.ConditionFalse, "MarshalFailed", err.Error()); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, nil
	}

	if err := r.upsertConfigMap(ctx, r.operatorNamespace(pool.Namespace), string(configJSON)); err != nil {
		err = fmt.Errorf("upsert configmap: %w", err)
		logger.Error(err, "Could not update config map")
		r.recordEvent(&pool, corev1.EventTypeWarning, "ConfigMapUpdateFailed", err.Error())
		if statusErr := r.setReadyCondition(ctx, &pool, metav1.ConditionFalse, "ConfigMapUpdateFailed", err.Error()); statusErr != nil {
			return ctrl.Result{}, statusErr
		}
		return ctrl.Result{}, err
	}

	r.recordEvent(&pool, corev1.EventTypeNormal, "ConfigRendered", "Rendered engine configuration")
	if err := r.setReadyCondition(ctx, &pool, metav1.ConditionTrue, "Ready", "Configuration rendered successfully"); err != nil {
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

	// Sort by name for deterministic selection when multiple pools exist.
	sort.Slice(pools.Items, func(i, j int) bool {
		return pools.Items[i].Name < pools.Items[j].Name
	})

	if len(pools.Items) > 1 {
		logger.Info("multiple DNSUpstreamPools in namespace, using first alphabetically",
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

	return r.upsertConfigMap(ctx, configNamespace, string(configJSON))
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

func (r *DNSUpstreamPoolReconciler) getDefaultCacheProfile(ctx context.Context, namespace string) (*v1alpha1.DNSCacheProfile, error) {
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

	for i, upstream := range pool.Spec.Upstreams {
		if !isValidUpstreamAddress(upstream.Address) {
			return fmt.Errorf("spec.upstreams[%d].address %q is not a valid IP or DNS name", i, upstream.Address)
		}
		if upstream.Port < 0 || upstream.Port > 65535 {
			return fmt.Errorf("spec.upstreams[%d].port must be between 0 and 65535", i)
		}
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

	return len(validation.IsDNS1123Subdomain(trimmed)) == 0
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
	r.Recorder.Event(object, eventType, reason, message)
}

var _ reconcile.Reconciler = (*DNSUpstreamPoolReconciler)(nil)
