package engineconfig

import (
	"errors"
	"fmt"
	"net/netip"
	"runtime"
	"sort"
	"strings"

	v1alpha1 "github.com/astradns/astradns-types/api/v1alpha1"
	"github.com/astradns/astradns-types/engine"
	typesengineconfig "github.com/astradns/astradns-types/engineconfig"
)

const (
	defaultUpstreamPortDNS     = 53
	defaultUpstreamPortDoT     = 853
	defaultUpstreamPortDoH     = 443
	defaultUpstreamWeight      = 1
	defaultUpstreamPreference  = 100
	defaultCacheMaxEntries     = 100000
	defaultPositiveTTLMin      = 60
	defaultPositiveTTLMax      = 300
	defaultNegativeTTL         = 30
	defaultPrefetchEnabled     = true
	defaultPrefetchThreshold   = 10
	defaultEngineListenAddress = "127.0.0.1"
	defaultEngineListeningPort = 5354
	defaultDNSSECMode          = engine.DNSSECModeOff
	defaultWorkerThreads       = int32(2)
	maxWorkerThreads           = int32(256)
)

// DefaultConfigGenerator converts CRD objects to an engine-agnostic EngineConfig.
type DefaultConfigGenerator struct{}

// Verify interface compliance at compile time.
var _ typesengineconfig.ConfigGenerator = (*DefaultConfigGenerator)(nil)

// Generate maps CRD fields to EngineConfig, applying defaults for empty fields.
func (g *DefaultConfigGenerator) Generate(
	pool *v1alpha1.DNSUpstreamPool,
	profile *v1alpha1.DNSCacheProfile,
) (*engine.EngineConfig, error) {
	if pool == nil {
		return nil, errors.New("dns upstream pool is required")
	}

	if len(pool.Spec.Upstreams) == 0 {
		return nil, errors.New("dns upstream pool must contain at least one upstream")
	}

	config := &engine.EngineConfig{
		Cache: engine.CacheConfig{
			MaxEntries:        defaultCacheMaxEntries,
			PositiveTtlMin:    defaultPositiveTTLMin,
			PositiveTtlMax:    defaultPositiveTTLMax,
			NegativeTtl:       defaultNegativeTTL,
			PrefetchEnabled:   defaultPrefetchEnabled,
			PrefetchThreshold: defaultPrefetchThreshold,
		},
		ListenAddr:    defaultEngineListenAddress,
		ListenPort:    defaultEngineListeningPort,
		WorkerThreads: resolveWorkerThreads(pool.Spec.Runtime.WorkerThreads),
		DNSSEC:        engine.DNSSECConfig{Mode: normalizeDNSSECMode(pool.Spec.DNSSEC.Mode)},
	}

	upstreams := make([]v1alpha1.Upstream, len(pool.Spec.Upstreams))
	copy(upstreams, pool.Spec.Upstreams)
	sort.SliceStable(upstreams, func(i, j int) bool {
		leftPreference := normalizeUpstreamPreference(upstreams[i].Preference)
		rightPreference := normalizeUpstreamPreference(upstreams[j].Preference)
		if leftPreference != rightPreference {
			return leftPreference < rightPreference
		}

		leftWeight := normalizeUpstreamWeight(upstreams[i].Weight)
		rightWeight := normalizeUpstreamWeight(upstreams[j].Weight)
		return leftWeight > rightWeight
	})

	for i, upstream := range upstreams {
		address := strings.TrimSpace(upstream.Address)
		if address == "" {
			return nil, fmt.Errorf("upstream at index %d has empty address", i)
		}

		transport := normalizeUpstreamTransport(upstream.Transport)

		port := upstream.Port
		if port == 0 {
			port = defaultUpstreamPortForTransport(transport)
		}

		tlsServerName := strings.TrimSpace(upstream.TLSServerName)
		if transport != engine.UpstreamTransportDNS && tlsServerName == "" {
			tlsServerName = defaultTLSServerName(address)
		}
		if transport == engine.UpstreamTransportDNS {
			tlsServerName = ""
		}

		config.Upstreams = append(config.Upstreams, engine.UpstreamConfig{
			Address:       address,
			Port:          port,
			Transport:     transport,
			TLSServerName: tlsServerName,
			Weight:        normalizeUpstreamWeight(upstream.Weight),
			Preference:    normalizeUpstreamPreference(upstream.Preference),
		})
	}

	if profile == nil {
		return config, nil
	}

	if profile.Spec.MaxEntries > 0 {
		config.Cache.MaxEntries = profile.Spec.MaxEntries
	}
	if profile.Spec.PositiveTtl.MinSeconds > 0 {
		config.Cache.PositiveTtlMin = profile.Spec.PositiveTtl.MinSeconds
	}
	if profile.Spec.PositiveTtl.MaxSeconds > 0 {
		config.Cache.PositiveTtlMax = profile.Spec.PositiveTtl.MaxSeconds
	}
	if profile.Spec.NegativeTtl.Seconds > 0 {
		config.Cache.NegativeTtl = profile.Spec.NegativeTtl.Seconds
	}

	config.Cache.PrefetchEnabled = profile.Spec.Prefetch.Enabled
	if profile.Spec.Prefetch.Threshold > 0 {
		config.Cache.PrefetchThreshold = profile.Spec.Prefetch.Threshold
	}

	return config, nil
}

func defaultUpstreamPortForTransport(transport engine.UpstreamTransport) int32 {
	switch transport {
	case engine.UpstreamTransportDoT:
		return defaultUpstreamPortDoT
	case engine.UpstreamTransportDoH:
		return defaultUpstreamPortDoH
	default:
		return defaultUpstreamPortDNS
	}
}

func normalizeUpstreamTransport(transport string) engine.UpstreamTransport {
	trimmed := strings.ToLower(strings.TrimSpace(transport))
	switch engine.UpstreamTransport(trimmed) {
	case engine.UpstreamTransportDoT:
		return engine.UpstreamTransportDoT
	case engine.UpstreamTransportDoH:
		return engine.UpstreamTransportDoH
	default:
		return engine.UpstreamTransportDNS
	}
}

func normalizeDNSSECMode(mode string) engine.DNSSECMode {
	trimmed := strings.ToLower(strings.TrimSpace(mode))
	switch engine.DNSSECMode(trimmed) {
	case engine.DNSSECModeProcess:
		return engine.DNSSECModeProcess
	case engine.DNSSECModeValidate:
		return engine.DNSSECModeValidate
	default:
		return defaultDNSSECMode
	}
}

func normalizeUpstreamWeight(weight int32) int32 {
	if weight <= 0 {
		return defaultUpstreamWeight
	}
	return weight
}

func normalizeUpstreamPreference(preference int32) int32 {
	if preference <= 0 {
		return defaultUpstreamPreference
	}
	return preference
}

func resolveWorkerThreads(value int32) int32 {
	if value > maxWorkerThreads {
		return maxWorkerThreads
	}
	if value > 0 {
		return value
	}

	auto := int32(runtime.NumCPU())
	if auto <= 0 {
		return defaultWorkerThreads
	}
	if auto > maxWorkerThreads {
		return maxWorkerThreads
	}

	return auto
}

func defaultTLSServerName(address string) string {
	trimmed := strings.TrimSuffix(strings.TrimSpace(address), ".")
	if trimmed == "" {
		return ""
	}
	if _, err := netip.ParseAddr(trimmed); err == nil {
		return ""
	}

	return trimmed
}
