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
		ListenAddr:    "0.0.0.0",
		ListenPort:    5354,
		WorkerThreads: 2,
		DNSSEC:        engine.DNSSECConfig{Mode: engine.DNSSECModeOff},
	}

	got, err := renderer.Render(config)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	expected := `server:
    interface: 0.0.0.0
    port: 5354
    username: ""
    chroot: ""
    do-daemonize: no
    use-syslog: no
    log-queries: no
    msg-cache-size: 4m
    rrset-cache-size: 8m
    cache-min-ttl: 30
    cache-max-ttl: 120
    cache-max-negative-ttl: 20
    prefetch: yes
    module-config: "iterator"
    val-permissive-mode: yes
    serve-expired: yes
    num-threads: 2
    access-control: 127.0.0.0/8 allow
    access-control: 10.0.0.0/8 allow
    access-control: 172.16.0.0/12 allow
    access-control: 192.168.0.0/16 allow

forward-zone:
    name: "."
    forward-tls-upstream: no
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
		ListenAddr:    "127.0.0.1",
		ListenPort:    5354,
		WorkerThreads: 2,
	}

	got, err := renderer.Render(config)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	checks := []string{
		"interface: 127.0.0.1",
		"port: 5354",
		"prefetch: no",
		"forward-tls-upstream: no",
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
		ListenAddr:    "127.0.0.1",
		ListenPort:    5354,
		WorkerThreads: 2,
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
			Runtime: v1alpha1.RuntimeConfig{WorkerThreads: 2},
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
    username: ""
    chroot: ""
    do-daemonize: no
    use-syslog: no
    log-queries: no
    msg-cache-size: 2m
    rrset-cache-size: 4m
    cache-min-ttl: 45
    cache-max-ttl: 180
    cache-max-negative-ttl: 20
    prefetch: yes
    module-config: "iterator"
    val-permissive-mode: yes
    serve-expired: yes
    num-threads: 2
    access-control: 127.0.0.0/8 allow
    access-control: 10.0.0.0/8 allow
    access-control: 172.16.0.0/12 allow
    access-control: 192.168.0.0/16 allow

forward-zone:
    name: "."
    forward-tls-upstream: no
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

// validUnboundConfig returns a minimal valid EngineConfig for Unbound rendering tests.
// Callers should override only the fields relevant to their test scenario.
func validUnboundConfig() *engine.EngineConfig {
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
		ListenAddr:    "127.0.0.1",
		ListenPort:    5354,
		WorkerThreads: 2,
		DNSSEC:        engine.DNSSECConfig{Mode: engine.DNSSECModeOff},
	}
}

// --- Gap 9: IPv6 upstream addresses in rendering ---

func TestUnboundRendererIPv6Upstreams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		upstreams []engine.UpstreamConfig
		checks    []string
		absent    []string
	}{
		{
			name: "single IPv6 loopback with port 53",
			upstreams: []engine.UpstreamConfig{
				{Address: "::1", Port: 53},
			},
			checks: []string{
				"forward-addr: ::1",
			},
			absent: []string{
				"forward-addr: ::1@53",
			},
		},
		{
			name: "IPv6 upstream with non-standard port 5353",
			upstreams: []engine.UpstreamConfig{
				{Address: "2001:db8::1", Port: 5353},
			},
			checks: []string{
				"forward-addr: 2001:db8::1@5353",
			},
		},
		{
			name: "mixed IPv4 and IPv6 upstreams",
			upstreams: []engine.UpstreamConfig{
				{Address: "1.1.1.1", Port: 53},
				{Address: "2001:db8::1", Port: 5353},
			},
			checks: []string{
				"forward-addr: 1.1.1.1",
				"forward-addr: 2001:db8::1@5353",
			},
			absent: []string{
				"forward-addr: 1.1.1.1@53",
			},
		},
	}

	renderer := &UnboundRenderer{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := validUnboundConfig()
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
			for _, check := range tt.absent {
				if strings.Contains(got, check) {
					t.Fatalf("Render() output should not contain %q\nfull output:\n%s", check, got)
				}
			}
		})
	}
}

// --- Gap 12: Empty upstreams list in rendering ---

func TestUnboundRendererEmptyUpstreams(t *testing.T) {
	t.Parallel()

	renderer := &UnboundRenderer{}
	config := validUnboundConfig()
	config.Upstreams = []engine.UpstreamConfig{}

	got, err := renderer.Render(config)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	// With no upstreams, the forward-zone section should still be present
	// but no forward-addr lines should appear.
	if !strings.Contains(got, "forward-zone:") {
		t.Fatalf("Render() output should contain forward-zone section\nfull output:\n%s", got)
	}
	if strings.Contains(got, "forward-addr:") {
		t.Fatalf("Render() output should not contain forward-addr when upstreams are empty\nfull output:\n%s", got)
	}
}

func TestUnboundRendererUpstreamPortZeroNormalization(t *testing.T) {
	t.Parallel()

	renderer := &UnboundRenderer{}
	config := validUnboundConfig()
	config.Upstreams = []engine.UpstreamConfig{
		{Address: "9.9.9.9", Port: 0},
	}

	got, err := renderer.Render(config)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	if !strings.Contains(got, "forward-addr: 9.9.9.9") {
		t.Fatalf("Render() output does not contain normalized upstream\nfull output:\n%s", got)
	}
	if strings.Contains(got, "forward-addr: 9.9.9.9@0") {
		t.Fatalf("Render() output should normalize port 0 to 53, not render @0\nfull output:\n%s", got)
	}
	if strings.Contains(got, "forward-addr: 9.9.9.9@53") {
		t.Fatalf("Render() output should omit @53 for default port\nfull output:\n%s", got)
	}
}

// --- Gap 14: Zero/negative cache values in rendering ---

func TestUnboundRendererZeroCacheValues(t *testing.T) {
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
				"msg-cache-size: 0k",
				"rrset-cache-size: 0k",
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
				"cache-max-negative-ttl: 0",
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
				"cache-min-ttl: 0",
				"cache-max-ttl: 0",
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
				"msg-cache-size:",
				"rrset-cache-size:",
			},
		},
	}

	renderer := &UnboundRenderer{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := validUnboundConfig()
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

func TestUnboundRendererVeryLargeMaxEntriesCapped(t *testing.T) {
	t.Parallel()

	renderer := &UnboundRenderer{}
	config := validUnboundConfig()
	config.Cache.MaxEntries = 10000000

	got, err := renderer.Render(config)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	// 10,000,000 * 1024 bytes = 10,240,000k = 10000m which exceeds the 256m cap.
	// msg-cache-size should be capped at 256m, rrset-cache-size at 512m.
	if !strings.Contains(got, "msg-cache-size: 256m") {
		t.Fatalf("Render() should cap msg-cache-size at 256m for very large MaxEntries\nfull output:\n%s", got)
	}
	if !strings.Contains(got, "rrset-cache-size: 512m") {
		t.Fatalf("Render() should cap rrset-cache-size at 512m for very large MaxEntries\nfull output:\n%s", got)
	}
}

func TestUnboundRendererSupportsDoTUpstreams(t *testing.T) {
	t.Parallel()

	renderer := &UnboundRenderer{}
	config := validUnboundConfig()
	config.Upstreams = []engine.UpstreamConfig{
		{Address: "dns.quad9.net", Transport: engine.UpstreamTransportDoT},
	}

	got, err := renderer.Render(config)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	if !strings.Contains(got, "forward-tls-upstream: yes") {
		t.Fatalf("expected unbound forward-tls-upstream to be enabled\n%s", got)
	}
	if !strings.Contains(got, "forward-addr: dns.quad9.net@853") {
		t.Fatalf("expected DoT upstream with default 853 port\n%s", got)
	}
}

func TestUnboundRendererRejectsDoHUpstreams(t *testing.T) {
	t.Parallel()

	renderer := &UnboundRenderer{}
	config := validUnboundConfig()
	config.Upstreams = []engine.UpstreamConfig{{Address: "dns.google", Transport: engine.UpstreamTransportDoH}}

	if _, err := renderer.Render(config); err == nil {
		t.Fatal("expected error for DoH upstream in unbound renderer")
	}
}
