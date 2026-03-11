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
