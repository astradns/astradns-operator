package engineconfig

import (
	"reflect"
	"testing"

	v1alpha1 "github.com/astradns/astradns-types/api/v1alpha1"
	"github.com/astradns/astradns-types/engine"
	typesengineconfig "github.com/astradns/astradns-types/engineconfig"

	_ "github.com/astradns/astradns-operator/pkg/engineconfig/coredns"
	_ "github.com/astradns/astradns-operator/pkg/engineconfig/powerdns"
	_ "github.com/astradns/astradns-operator/pkg/engineconfig/unbound"
)

func TestDefaultConfigGeneratorGenerateWithDefaults(t *testing.T) {
	t.Parallel()

	gen := &DefaultConfigGenerator{}
	pool := &v1alpha1.DNSUpstreamPool{
		Spec: v1alpha1.DNSUpstreamPoolSpec{
			Upstreams: []v1alpha1.Upstream{
				{Address: "1.1.1.1"},
				{Address: "8.8.8.8", Port: 5353, Transport: "dns", Weight: 5, Preference: 10},
			},
			DNSSEC: v1alpha1.DNSSECConfig{Mode: "process"},
			Runtime: v1alpha1.RuntimeConfig{
				WorkerThreads: 6,
			},
		},
	}

	got, err := gen.Generate(pool, nil)
	if err != nil {
		t.Fatalf("Generate() returned error: %v", err)
	}

	want := &engine.EngineConfig{
		Upstreams: []engine.UpstreamConfig{
			{Address: "8.8.8.8", Port: 5353, Transport: engine.UpstreamTransportDNS, Weight: 5, Preference: 10},
			{Address: "1.1.1.1", Port: 53, Transport: engine.UpstreamTransportDNS, Weight: 1, Preference: 100},
		},
		Cache: engine.CacheConfig{
			MaxEntries:        100000,
			PositiveTtlMin:    60,
			PositiveTtlMax:    300,
			NegativeTtl:       30,
			PrefetchEnabled:   true,
			PrefetchThreshold: 10,
		},
		ListenAddr:    "127.0.0.1",
		ListenPort:    5354,
		WorkerThreads: 6,
		DNSSEC: engine.DNSSECConfig{
			Mode: engine.DNSSECModeProcess,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Generate() mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestDefaultConfigGeneratorGenerateWithProfileOverrides(t *testing.T) {
	t.Parallel()

	gen := &DefaultConfigGenerator{}
	pool := &v1alpha1.DNSUpstreamPool{
		Spec: v1alpha1.DNSUpstreamPoolSpec{
			Upstreams: []v1alpha1.Upstream{{Address: "9.9.9.9"}},
		},
	}
	profile := &v1alpha1.DNSCacheProfile{
		Spec: v1alpha1.DNSCacheProfileSpec{
			MaxEntries: 2000,
			PositiveTtl: v1alpha1.TtlConfig{
				MinSeconds: 25,
				MaxSeconds: 90,
			},
			NegativeTtl: v1alpha1.NegTtlConfig{Seconds: 15},
			Prefetch:    v1alpha1.PrefetchConfig{Enabled: true, Threshold: 5},
		},
	}

	got, err := gen.Generate(pool, profile)
	if err != nil {
		t.Fatalf("Generate() returned error: %v", err)
	}

	if got.Cache.MaxEntries != 2000 {
		t.Fatalf("MaxEntries = %d, want %d", got.Cache.MaxEntries, 2000)
	}
	if got.Cache.PositiveTtlMin != 25 {
		t.Fatalf("PositiveTtlMin = %d, want %d", got.Cache.PositiveTtlMin, 25)
	}
	if got.Cache.PositiveTtlMax != 90 {
		t.Fatalf("PositiveTtlMax = %d, want %d", got.Cache.PositiveTtlMax, 90)
	}
	if got.Cache.NegativeTtl != 15 {
		t.Fatalf("NegativeTtl = %d, want %d", got.Cache.NegativeTtl, 15)
	}
	if !got.Cache.PrefetchEnabled {
		t.Fatal("PrefetchEnabled = false, want true")
	}
	if got.Cache.PrefetchThreshold != 5 {
		t.Fatalf("PrefetchThreshold = %d, want %d", got.Cache.PrefetchThreshold, 5)
	}
	if got.WorkerThreads <= 0 {
		t.Fatalf("WorkerThreads = %d, want positive value", got.WorkerThreads)
	}
	if got.DNSSEC.Mode != engine.DNSSECModeOff {
		t.Fatalf("DNSSEC mode = %q, want %q", got.DNSSEC.Mode, engine.DNSSECModeOff)
	}
}

func TestDefaultConfigGeneratorGenerateNilPool(t *testing.T) {
	t.Parallel()

	gen := &DefaultConfigGenerator{}
	if _, err := gen.Generate(nil, nil); err == nil {
		t.Fatal("Generate() error = nil, want non-nil")
	}
}

// --- Gap 11: Profile with Prefetch.Enabled=false overriding default ---

func TestDefaultConfigGeneratorPrefetchDisabledOverridesDefault(t *testing.T) {
	t.Parallel()

	gen := &DefaultConfigGenerator{}
	pool := &v1alpha1.DNSUpstreamPool{
		Spec: v1alpha1.DNSUpstreamPoolSpec{
			Upstreams: []v1alpha1.Upstream{{Address: "1.1.1.1"}},
		},
	}
	profile := &v1alpha1.DNSCacheProfile{
		Spec: v1alpha1.DNSCacheProfileSpec{
			Prefetch: v1alpha1.PrefetchConfig{Enabled: false, Threshold: 0},
		},
	}

	got, err := gen.Generate(pool, profile)
	if err != nil {
		t.Fatalf("Generate() returned error: %v", err)
	}

	if got.Cache.PrefetchEnabled {
		t.Fatal("PrefetchEnabled = true, want false when profile explicitly sets Enabled=false")
	}
	// Threshold should remain at default because profile value is 0
	if got.Cache.PrefetchThreshold != 10 {
		t.Fatalf("PrefetchThreshold = %d, want %d (default)", got.Cache.PrefetchThreshold, 10)
	}
}

func TestDefaultConfigGeneratorOnlyMaxEntriesOverridden(t *testing.T) {
	t.Parallel()

	gen := &DefaultConfigGenerator{}
	pool := &v1alpha1.DNSUpstreamPool{
		Spec: v1alpha1.DNSUpstreamPoolSpec{
			Upstreams: []v1alpha1.Upstream{{Address: "1.1.1.1"}},
		},
	}
	profile := &v1alpha1.DNSCacheProfile{
		Spec: v1alpha1.DNSCacheProfileSpec{
			MaxEntries: 5000,
		},
	}

	got, err := gen.Generate(pool, profile)
	if err != nil {
		t.Fatalf("Generate() returned error: %v", err)
	}

	if got.Cache.MaxEntries != 5000 {
		t.Fatalf("MaxEntries = %d, want %d", got.Cache.MaxEntries, 5000)
	}
	// All other fields should retain their defaults
	if got.Cache.PositiveTtlMin != 60 {
		t.Fatalf("PositiveTtlMin = %d, want %d (default)", got.Cache.PositiveTtlMin, 60)
	}
	if got.Cache.PositiveTtlMax != 300 {
		t.Fatalf("PositiveTtlMax = %d, want %d (default)", got.Cache.PositiveTtlMax, 300)
	}
	if got.Cache.NegativeTtl != 30 {
		t.Fatalf("NegativeTtl = %d, want %d (default)", got.Cache.NegativeTtl, 30)
	}
	// Prefetch.Enabled=false in zero-value profile overrides default true
	if got.Cache.PrefetchEnabled {
		t.Fatal("PrefetchEnabled = true, want false when profile Prefetch.Enabled is zero-value false")
	}
	if got.Cache.PrefetchThreshold != 10 {
		t.Fatalf("PrefetchThreshold = %d, want %d (default)", got.Cache.PrefetchThreshold, 10)
	}
}

func TestDefaultConfigGeneratorAllZeroValueProfile(t *testing.T) {
	t.Parallel()

	gen := &DefaultConfigGenerator{}
	pool := &v1alpha1.DNSUpstreamPool{
		Spec: v1alpha1.DNSUpstreamPoolSpec{
			Upstreams: []v1alpha1.Upstream{{Address: "1.1.1.1"}},
		},
	}
	profile := &v1alpha1.DNSCacheProfile{
		Spec: v1alpha1.DNSCacheProfileSpec{},
	}

	got, err := gen.Generate(pool, profile)
	if err != nil {
		t.Fatalf("Generate() returned error: %v", err)
	}

	// All zero-value profile fields should result in defaults being kept,
	// except PrefetchEnabled which is always directly assigned from profile.
	if got.Cache.MaxEntries != 100000 {
		t.Fatalf("MaxEntries = %d, want %d (default)", got.Cache.MaxEntries, 100000)
	}
	if got.Cache.PositiveTtlMin != 60 {
		t.Fatalf("PositiveTtlMin = %d, want %d (default)", got.Cache.PositiveTtlMin, 60)
	}
	if got.Cache.PositiveTtlMax != 300 {
		t.Fatalf("PositiveTtlMax = %d, want %d (default)", got.Cache.PositiveTtlMax, 300)
	}
	if got.Cache.NegativeTtl != 30 {
		t.Fatalf("NegativeTtl = %d, want %d (default)", got.Cache.NegativeTtl, 30)
	}
	// PrefetchEnabled is directly assigned: profile zero-value false overrides default true
	if got.Cache.PrefetchEnabled {
		t.Fatal("PrefetchEnabled = true, want false when profile has zero-value (false)")
	}
	if got.Cache.PrefetchThreshold != 10 {
		t.Fatalf("PrefetchThreshold = %d, want %d (default)", got.Cache.PrefetchThreshold, 10)
	}
}

func TestDefaultConfigGeneratorTransportPortsAndTLSDefaults(t *testing.T) {
	t.Parallel()

	gen := &DefaultConfigGenerator{}
	pool := &v1alpha1.DNSUpstreamPool{
		Spec: v1alpha1.DNSUpstreamPoolSpec{
			Upstreams: []v1alpha1.Upstream{
				{Address: "1.1.1.1"},
				{Address: "dns.quad9.net", Transport: "dot"},
				{Address: "dns.google", Transport: "doh"},
			},
		},
	}

	got, err := gen.Generate(pool, nil)
	if err != nil {
		t.Fatalf("Generate() returned error: %v", err)
	}

	if got.Upstreams[0].Port != 53 {
		t.Fatalf("first upstream port = %d, want 53", got.Upstreams[0].Port)
	}
	if got.Upstreams[1].Port != 853 {
		t.Fatalf("dot upstream port = %d, want 853", got.Upstreams[1].Port)
	}
	if got.Upstreams[2].Port != 443 {
		t.Fatalf("doh upstream port = %d, want 443", got.Upstreams[2].Port)
	}
	if got.Upstreams[1].TLSServerName != "dns.quad9.net" {
		t.Fatalf("dot tlsServerName = %q, want dns.quad9.net", got.Upstreams[1].TLSServerName)
	}
	if got.Upstreams[2].TLSServerName != "dns.google" {
		t.Fatalf("doh tlsServerName = %q, want dns.google", got.Upstreams[2].TLSServerName)
	}
}

func TestRendererRegistration(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		engineType engine.EngineType
		configFile string
	}{
		{engineType: engine.EngineUnbound, configFile: "unbound.conf"},
		{engineType: engine.EngineCoreDNS, configFile: "Corefile"},
		{engineType: engine.EnginePowerDNS, configFile: "recursor.conf"},
	}

	for _, tc := range testCases {
		renderer, err := typesengineconfig.NewRenderer(tc.engineType)
		if err != nil {
			t.Fatalf("NewRenderer(%q) returned error: %v", tc.engineType, err)
		}
		if renderer.EngineType() != tc.engineType {
			t.Fatalf("EngineType() = %q, want %q", renderer.EngineType(), tc.engineType)
		}
		if renderer.ConfigFileName() != tc.configFile {
			t.Fatalf("ConfigFileName() = %q, want %q", renderer.ConfigFileName(), tc.configFile)
		}
	}
}
