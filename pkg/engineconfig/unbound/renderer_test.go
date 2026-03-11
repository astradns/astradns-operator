package unbound

import (
	"strings"
	"testing"

	operatorconfig "github.com/astradns/astradns-operator/pkg/engineconfig"
	v1alpha1 "github.com/astradns/astradns-types/api/v1alpha1"
	"github.com/astradns/astradns-types/engine"
)

func TestUnboundRendererRenderFullConfig(t *testing.T) {
	t.Parallel()

	renderer := &UnboundRenderer{}
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

	expected := `server:
    interface: 0.0.0.0
    port: 5354
    do-daemonize: no
    use-syslog: no
    log-queries: no
    msg-cache-size: 4m
    rrset-cache-size: 8m
    cache-min-ttl: 30
    cache-max-ttl: 120
    cache-max-negative-ttl: 20
    prefetch: yes
    serve-expired: yes
    num-threads: 2
    access-control: 127.0.0.0/8 allow
    access-control: 10.0.0.0/8 allow
    access-control: 172.16.0.0/12 allow
    access-control: 192.168.0.0/16 allow

forward-zone:
    name: "."
    forward-addr: 1.1.1.1
    forward-addr: 8.8.8.8@5353`

	if strings.TrimSpace(got) != strings.TrimSpace(expected) {
		t.Fatalf("Render() output mismatch\nexpected:\n%s\n\ngot:\n%s", expected, got)
	}
}

func TestUnboundRendererRenderDefaults(t *testing.T) {
	t.Parallel()

	renderer := &UnboundRenderer{}
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
		"interface: 127.0.0.1",
		"port: 5354",
		"prefetch: no",
		"forward-addr: 1.1.1.1",
	}

	for _, check := range checks {
		if !strings.Contains(got, check) {
			t.Fatalf("Render() output does not contain %q\nfull output:\n%s", check, got)
		}
	}
}

func TestUnboundRendererRenderDefaultUpstreamPort(t *testing.T) {
	t.Parallel()

	renderer := &UnboundRenderer{}
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

	if !strings.Contains(got, "forward-addr: 1.1.1.1") {
		t.Fatalf("Render() output does not contain default upstream address\nfull output:\n%s", got)
	}
	if strings.Contains(got, "forward-addr: 1.1.1.1@0") {
		t.Fatalf("Render() output should normalize upstream port 0 to 53\nfull output:\n%s", got)
	}
}

func TestUnboundRendererRoundTrip(t *testing.T) {
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

	renderer := &UnboundRenderer{}
	got, err := renderer.Render(config)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	expected := `server:
    interface: 127.0.0.1
    port: 5354
    do-daemonize: no
    use-syslog: no
    log-queries: no
    msg-cache-size: 2m
    rrset-cache-size: 4m
    cache-min-ttl: 45
    cache-max-ttl: 180
    cache-max-negative-ttl: 20
    prefetch: yes
    serve-expired: yes
    num-threads: 2
    access-control: 127.0.0.0/8 allow
    access-control: 10.0.0.0/8 allow
    access-control: 172.16.0.0/12 allow
    access-control: 192.168.0.0/16 allow

forward-zone:
    name: "."
    forward-addr: 1.1.1.1
    forward-addr: 8.8.8.8@5353`

	if strings.TrimSpace(got) != strings.TrimSpace(expected) {
		t.Fatalf("round-trip output mismatch\nexpected:\n%s\n\ngot:\n%s", expected, got)
	}
}

func TestUnboundRendererMetadata(t *testing.T) {
	t.Parallel()

	renderer := &UnboundRenderer{}
	if renderer.EngineType() != engine.EngineUnbound {
		t.Fatalf("EngineType() = %q, want %q", renderer.EngineType(), engine.EngineUnbound)
	}
	if renderer.ConfigFileName() != "unbound.conf" {
		t.Fatalf("ConfigFileName() = %q, want %q", renderer.ConfigFileName(), "unbound.conf")
	}
}
