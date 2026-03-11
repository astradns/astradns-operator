package engineconfig

import (
	"errors"
	"fmt"
	"strings"

	v1alpha1 "github.com/astradns/astradns-types/api/v1alpha1"
	"github.com/astradns/astradns-types/engine"
	typesengineconfig "github.com/astradns/astradns-types/engineconfig"
)

const (
	defaultUpstreamPort        = 53
	defaultCacheMaxEntries     = 100000
	defaultPositiveTTLMin      = 60
	defaultPositiveTTLMax      = 300
	defaultNegativeTTL         = 30
	defaultPrefetchThreshold   = 10
	defaultEngineListenAddress = "127.0.0.1"
	defaultEngineListeningPort = 5354
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
			PrefetchThreshold: defaultPrefetchThreshold,
		},
		ListenAddr: defaultEngineListenAddress,
		ListenPort: defaultEngineListeningPort,
	}

	for i, upstream := range pool.Spec.Upstreams {
		address := strings.TrimSpace(upstream.Address)
		if address == "" {
			return nil, fmt.Errorf("upstream at index %d has empty address", i)
		}

		port := int(upstream.Port)
		if port == 0 {
			port = defaultUpstreamPort
		}

		config.Upstreams = append(config.Upstreams, engine.UpstreamConfig{
			Address: address,
			Port:    port,
		})
	}

	if profile == nil {
		return config, nil
	}

	if profile.Spec.MaxEntries > 0 {
		config.Cache.MaxEntries = int(profile.Spec.MaxEntries)
	}
	if profile.Spec.PositiveTtl.MinSeconds > 0 {
		config.Cache.PositiveTtlMin = int(profile.Spec.PositiveTtl.MinSeconds)
	}
	if profile.Spec.PositiveTtl.MaxSeconds > 0 {
		config.Cache.PositiveTtlMax = int(profile.Spec.PositiveTtl.MaxSeconds)
	}
	if profile.Spec.NegativeTtl.Seconds > 0 {
		config.Cache.NegativeTtl = int(profile.Spec.NegativeTtl.Seconds)
	}

	config.Cache.PrefetchEnabled = profile.Spec.Prefetch.Enabled
	if profile.Spec.Prefetch.Threshold > 0 {
		config.Cache.PrefetchThreshold = int(profile.Spec.Prefetch.Threshold)
	}

	return config, nil
}
