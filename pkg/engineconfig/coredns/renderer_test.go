package coredns

import (
	"strings"
	"testing"

	operatorconfig "github.com/astradns/astradns-operator/pkg/engineconfig"
	v1alpha1 "github.com/astradns/astradns-types/api/v1alpha1"
	"github.com/astradns/astradns-types/engine"
)

func TestCoreDNSRendererRenderFullConfig(t *testing.T) {
	t.Parallel()

	renderer := &CoreDNSRenderer{}
	config := &engine.EngineConfig{
		Upstreams: []engine.UpstreamConfig{
			{Address: "1.1.1.1", Port: 53},
			{Address: "8.8.8.8", Port: 5353},
		},
		Cache: engine.CacheConfig{
			MaxEntries:        4096,
			PositiveTtlMin:    30,
			PositiveTtlMax:    120,
			NegativeTtl:       20,
			PrefetchEnabled:   true,
			PrefetchThreshold: 7,
		},
		ListenAddr: "0.0.0.0",
		ListenPort: 5354,
	}

	got, err := renderer.Render(config)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	expected := `.:5354 {
    bind 0.0.0.0
    forward . 1.1.1.1:53 8.8.8.8:5353 {
        policy sequential
    }
    cache 120 {
        success 4096 120
        denial 4096 20
        prefetch 7 1h 10%
    }
    reload 5s
    errors
    log
}`

	if strings.TrimSpace(got) != strings.TrimSpace(expected) {
		t.Fatalf("Render() output mismatch\nexpected:\n%s\n\ngot:\n%s", expected, got)
	}
}

func TestCoreDNSRendererRenderDefaults(t *testing.T) {
	t.Parallel()

	renderer := &CoreDNSRenderer{}
	config := &engine.EngineConfig{
		Upstreams: []engine.UpstreamConfig{{Address: "1.1.1.1", Port: 53}},
		Cache: engine.CacheConfig{
			MaxEntries:     100000,
			PositiveTtlMin: 60,
			PositiveTtlMax: 300,
			NegativeTtl:    30,
		},
		ListenAddr: "127.0.0.1",
		ListenPort: 5354,
	}

	got, err := renderer.Render(config)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	checks := []string{
		".:5354 {",
		"bind 127.0.0.1",
		"forward . 1.1.1.1:53 {",
		"cache 300 {",
	}

	for _, check := range checks {
		if !strings.Contains(got, check) {
			t.Fatalf("Render() output does not contain %q\nfull output:\n%s", check, got)
		}
	}
}

func TestCoreDNSRendererRenderDefaultUpstreamPort(t *testing.T) {
	t.Parallel()

	renderer := &CoreDNSRenderer{}
	config := &engine.EngineConfig{
		Upstreams: []engine.UpstreamConfig{{Address: "1.1.1.1"}},
		Cache: engine.CacheConfig{
			MaxEntries:     100000,
			PositiveTtlMin: 60,
			PositiveTtlMax: 300,
			NegativeTtl:    30,
		},
		ListenAddr: "127.0.0.1",
		ListenPort: 5354,
	}

	got, err := renderer.Render(config)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	if !strings.Contains(got, "forward . 1.1.1.1:53 {") {
		t.Fatalf("Render() output does not contain normalized upstream port\nfull output:\n%s", got)
	}
	if strings.Contains(got, "1.1.1.1:0") {
		t.Fatalf("Render() output should normalize upstream port 0 to 53\nfull output:\n%s", got)
	}
}

func TestCoreDNSRendererRoundTrip(t *testing.T) {
	t.Parallel()

	gen := &operatorconfig.DefaultConfigGenerator{}
	pool := &v1alpha1.DNSUpstreamPool{
		Spec: v1alpha1.DNSUpstreamPoolSpec{
			Upstreams: []v1alpha1.Upstream{
				{Address: "1.1.1.1"},
				{Address: "8.8.8.8", Port: 5353},
			},
		},
	}
	profile := &v1alpha1.DNSCacheProfile{
		Spec: v1alpha1.DNSCacheProfileSpec{
			MaxEntries: 2048,
			PositiveTtl: v1alpha1.TtlConfig{
				MinSeconds: 45,
				MaxSeconds: 180,
			},
			NegativeTtl: v1alpha1.NegTtlConfig{Seconds: 20},
			Prefetch:    v1alpha1.PrefetchConfig{Enabled: true, Threshold: 8},
		},
	}

	config, err := gen.Generate(pool, profile)
	if err != nil {
		t.Fatalf("Generate() returned error: %v", err)
	}

	renderer := &CoreDNSRenderer{}
	got, err := renderer.Render(config)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	expected := `.:5354 {
    bind 127.0.0.1
    forward . 1.1.1.1:53 8.8.8.8:5353 {
        policy sequential
    }
    cache 180 {
        success 2048 180
        denial 2048 20
        prefetch 8 1h 10%
    }
    reload 5s
    errors
    log
}`

	if strings.TrimSpace(got) != strings.TrimSpace(expected) {
		t.Fatalf("round-trip output mismatch\nexpected:\n%s\n\ngot:\n%s", expected, got)
	}
}

func TestCoreDNSRendererMetadata(t *testing.T) {
	t.Parallel()

	renderer := &CoreDNSRenderer{}
	if renderer.EngineType() != engine.EngineCoreDNS {
		t.Fatalf("EngineType() = %q, want %q", renderer.EngineType(), engine.EngineCoreDNS)
	}
	if renderer.ConfigFileName() != "Corefile" {
		t.Fatalf("ConfigFileName() = %q, want %q", renderer.ConfigFileName(), "Corefile")
	}
}

// validCoreDNSConfig returns a minimal valid EngineConfig for CoreDNS rendering tests.
// Callers should override only the fields relevant to their test scenario.
func validCoreDNSConfig() *engine.EngineConfig {
	return &engine.EngineConfig{
		Upstreams: []engine.UpstreamConfig{
			{Address: "1.1.1.1", Port: 53},
		},
		Cache: engine.CacheConfig{
			MaxEntries:     100000,
			PositiveTtlMin: 60,
			PositiveTtlMax: 300,
			NegativeTtl:    30,
		},
		ListenAddr: "127.0.0.1",
		ListenPort: 5354,
	}
}

// --- Gap 9: IPv6 upstream addresses in rendering ---

func TestCoreDNSRendererIPv6Upstreams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		upstreams []engine.UpstreamConfig
		checks    []string
	}{
		{
			name: "single IPv6 loopback with port 53",
			upstreams: []engine.UpstreamConfig{
				{Address: "::1", Port: 53},
			},
			checks: []string{
				"forward . ::1:53",
			},
		},
		{
			name: "IPv6 upstream with non-standard port 5353",
			upstreams: []engine.UpstreamConfig{
				{Address: "2001:db8::1", Port: 5353},
			},
			checks: []string{
				"forward . 2001:db8::1:5353",
			},
		},
		{
			name: "mixed IPv4 and IPv6 upstreams",
			upstreams: []engine.UpstreamConfig{
				{Address: "1.1.1.1", Port: 53},
				{Address: "2001:db8::1", Port: 5353},
			},
			checks: []string{
				"1.1.1.1:53",
				"2001:db8::1:5353",
			},
		},
	}

	renderer := &CoreDNSRenderer{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := validCoreDNSConfig()
			config.Upstreams = tt.upstreams

			got, err := renderer.Render(config)
			if err != nil {
				t.Fatalf("Render() returned error: %v", err)
			}

			for _, check := range tt.checks {
				if !strings.Contains(got, check) {
					t.Fatalf("Render() output does not contain %q\nfull output:\n%s", check, got)
				}
			}
		})
	}
}

// --- Gap 12: Empty upstreams list in rendering ---

func TestCoreDNSRendererEmptyUpstreams(t *testing.T) {
	t.Parallel()

	renderer := &CoreDNSRenderer{}
	config := validCoreDNSConfig()
	config.Upstreams = []engine.UpstreamConfig{}

	got, err := renderer.Render(config)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	// With no upstreams, the forward directive should still render but
	// without any upstream addresses listed.
	if !strings.Contains(got, "forward .") {
		t.Fatalf("Render() output should contain forward directive\nfull output:\n%s", got)
	}
}

func TestCoreDNSRendererUpstreamPortZeroNormalization(t *testing.T) {
	t.Parallel()

	renderer := &CoreDNSRenderer{}
	config := validCoreDNSConfig()
	config.Upstreams = []engine.UpstreamConfig{
		{Address: "9.9.9.9", Port: 0},
	}

	got, err := renderer.Render(config)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	if !strings.Contains(got, "forward . 9.9.9.9:53") {
		t.Fatalf("Render() output should normalize port 0 to 53\nfull output:\n%s", got)
	}
	if strings.Contains(got, "9.9.9.9:0") {
		t.Fatalf("Render() output should not contain port 0\nfull output:\n%s", got)
	}
}

// --- Gap 14: Zero/negative cache values in rendering ---

func TestCoreDNSRendererZeroCacheValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cache  engine.CacheConfig
		checks []string
	}{
		{
			name: "MaxEntries zero",
			cache: engine.CacheConfig{
				MaxEntries:     0,
				PositiveTtlMin: 60,
				PositiveTtlMax: 300,
				NegativeTtl:    30,
			},
			checks: []string{
				"success 0 300",
				"denial 0 30",
			},
		},
		{
			name: "NegativeTtl zero",
			cache: engine.CacheConfig{
				MaxEntries:     100000,
				PositiveTtlMin: 60,
				PositiveTtlMax: 300,
				NegativeTtl:    0,
			},
			checks: []string{
				"denial 100000 0",
			},
		},
		{
			name: "PositiveTtlMin and PositiveTtlMax both zero",
			cache: engine.CacheConfig{
				MaxEntries:     100000,
				PositiveTtlMin: 0,
				PositiveTtlMax: 0,
				NegativeTtl:    30,
			},
			checks: []string{
				"cache 0 {",
				"success 100000 0",
			},
		},
		{
			name: "very large MaxEntries",
			cache: engine.CacheConfig{
				MaxEntries:     10000000,
				PositiveTtlMin: 60,
				PositiveTtlMax: 300,
				NegativeTtl:    30,
			},
			checks: []string{
				"success 10000000 300",
				"denial 10000000 30",
			},
		},
	}

	renderer := &CoreDNSRenderer{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := validCoreDNSConfig()
			config.Cache = tt.cache

			got, err := renderer.Render(config)
			if err != nil {
				t.Fatalf("Render() returned error: %v", err)
			}

			for _, check := range tt.checks {
				if !strings.Contains(got, check) {
					t.Fatalf("Render() output does not contain %q\nfull output:\n%s", check, got)
				}
			}
		})
	}
}

func TestCoreDNSRendererSupportsDoTAndDoHTargets(t *testing.T) {
	t.Parallel()

	renderer := &CoreDNSRenderer{}
	config := validCoreDNSConfig()
	config.Upstreams = []engine.UpstreamConfig{
		{Address: "dns.quad9.net", Transport: engine.UpstreamTransportDoT, TLSServerName: "resolver.example"},
		{Address: "dns.google", Transport: engine.UpstreamTransportDoH, TLSServerName: "resolver.example"},
	}

	got, err := renderer.Render(config)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	if !strings.Contains(got, "tls://dns.quad9.net:853") {
		t.Fatalf("expected DoT target in rendered Corefile\n%s", got)
	}
	if !strings.Contains(got, "https://dns.google:443") {
		t.Fatalf("expected DoH target in rendered Corefile\n%s", got)
	}
}

func TestCoreDNSRendererRejectsDNSSECMode(t *testing.T) {
	t.Parallel()

	renderer := &CoreDNSRenderer{}
	config := validCoreDNSConfig()
	config.DNSSEC.Mode = engine.DNSSECModeValidate

	if _, err := renderer.Render(config); err == nil {
		t.Fatal("expected DNSSEC mode validation error for CoreDNS renderer")
	}
}
