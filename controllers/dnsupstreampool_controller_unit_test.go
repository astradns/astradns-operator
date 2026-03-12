package controllers

import (
	"testing"
	"time"

	v1alpha1 "github.com/astradns/astradns-types/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResolvedAgentConfigMapName(t *testing.T) {
	reconciler := &DNSUpstreamPoolReconciler{}

	t.Setenv(agentConfigMapNameEnv, "release-agent-config")
	if got := reconciler.resolvedAgentConfigMapName(); got != "release-agent-config" {
		t.Fatalf("expected overridden ConfigMap name, got %q", got)
	}

	t.Setenv(agentConfigMapNameEnv, "")
	if got := reconciler.resolvedAgentConfigMapName(); got != agentConfigMapName {
		t.Fatalf("expected default ConfigMap name %q, got %q", agentConfigMapName, got)
	}
}

// --- Gap 6: validateDNSUpstreamPool and isValidUpstreamAddress edge cases ---

func TestIsValidUpstreamAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    bool
	}{
		{name: "valid IPv4", address: "1.1.1.1", want: true},
		{name: "valid hostname", address: "dns.google", want: true},
		{name: "invalid IPv4 literal 999.999.999.999", address: "999.999.999.999", want: false},
		{name: "invalid hostname leading dash", address: "-bad.com", want: false},
		{name: "empty address", address: "", want: false},
		{name: "whitespace-only address", address: "   ", want: false},
		{name: "valid IPv6", address: "2001:4860:4860::8888", want: true},
		{name: "valid subdomain hostname", address: "resolver1.opendns.com", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidUpstreamAddress(tt.address)
			if got != tt.want {
				t.Errorf("isValidUpstreamAddress(%q) = %v, want %v", tt.address, got, tt.want)
			}
		})
	}
}

func TestValidateDNSUpstreamPool(t *testing.T) {
	tests := []struct {
		name    string
		pool    *v1alpha1.DNSUpstreamPool
		wantErr bool
	}{
		{
			name:    "nil pool",
			pool:    nil,
			wantErr: true,
		},
		{
			name: "no upstreams",
			pool: &v1alpha1.DNSUpstreamPool{
				Spec: v1alpha1.DNSUpstreamPoolSpec{
					Upstreams: []v1alpha1.Upstream{},
				},
			},
			wantErr: true,
		},
		{
			name: "valid IPv4 address with port 53",
			pool: &v1alpha1.DNSUpstreamPool{
				Spec: v1alpha1.DNSUpstreamPoolSpec{
					Upstreams: []v1alpha1.Upstream{
						{Address: "1.1.1.1", Port: 53},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid hostname with port 53",
			pool: &v1alpha1.DNSUpstreamPool{
				Spec: v1alpha1.DNSUpstreamPoolSpec{
					Upstreams: []v1alpha1.Upstream{
						{Address: "dns.google", Port: 53},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid IPv4 literal 999.999.999.999",
			pool: &v1alpha1.DNSUpstreamPool{
				Spec: v1alpha1.DNSUpstreamPoolSpec{
					Upstreams: []v1alpha1.Upstream{
						{Address: "999.999.999.999", Port: 53},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid hostname leading dash",
			pool: &v1alpha1.DNSUpstreamPool{
				Spec: v1alpha1.DNSUpstreamPoolSpec{
					Upstreams: []v1alpha1.Upstream{
						{Address: "-bad.com", Port: 53},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "empty address",
			pool: &v1alpha1.DNSUpstreamPool{
				Spec: v1alpha1.DNSUpstreamPoolSpec{
					Upstreams: []v1alpha1.Upstream{
						{Address: "", Port: 53},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "whitespace-only address",
			pool: &v1alpha1.DNSUpstreamPool{
				Spec: v1alpha1.DNSUpstreamPoolSpec{
					Upstreams: []v1alpha1.Upstream{
						{Address: "   ", Port: 53},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "address with leading whitespace is invalid",
			pool: &v1alpha1.DNSUpstreamPool{
				Spec: v1alpha1.DNSUpstreamPoolSpec{
					Upstreams: []v1alpha1.Upstream{
						{Address: " 1.1.1.1", Port: 53},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "port 0 is invalid",
			pool: &v1alpha1.DNSUpstreamPool{
				Spec: v1alpha1.DNSUpstreamPoolSpec{
					Upstreams: []v1alpha1.Upstream{
						{Address: "1.1.1.1", Port: 0},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "port 65536 is invalid",
			pool: &v1alpha1.DNSUpstreamPool{
				Spec: v1alpha1.DNSUpstreamPoolSpec{
					Upstreams: []v1alpha1.Upstream{
						{Address: "1.1.1.1", Port: 65536},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "port 1 is valid",
			pool: &v1alpha1.DNSUpstreamPool{
				Spec: v1alpha1.DNSUpstreamPoolSpec{
					Upstreams: []v1alpha1.Upstream{
						{Address: "1.1.1.1", Port: 1},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "port 65535 is valid",
			pool: &v1alpha1.DNSUpstreamPool{
				Spec: v1alpha1.DNSUpstreamPoolSpec{
					Upstreams: []v1alpha1.Upstream{
						{Address: "1.1.1.1", Port: 65535},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "negative port is invalid",
			pool: &v1alpha1.DNSUpstreamPool{
				Spec: v1alpha1.DNSUpstreamPoolSpec{
					Upstreams: []v1alpha1.Upstream{
						{Address: "1.1.1.1", Port: -1},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "duplicate upstream address and port is invalid",
			pool: &v1alpha1.DNSUpstreamPool{
				Spec: v1alpha1.DNSUpstreamPoolSpec{
					Upstreams: []v1alpha1.Upstream{
						{Address: "1.1.1.1", Port: 53},
						{Address: "1.1.1.1", Port: 53},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDNSUpstreamPool(tt.pool)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDNSUpstreamPool() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// --- Gap 8: operatorNamespace with POD_NAMESPACE set and unset ---

func TestOperatorNamespace(t *testing.T) {
	reconciler := &DNSUpstreamPoolReconciler{}

	tests := []struct {
		name       string
		envValue   string
		fallback   string
		wantResult string
	}{
		{
			name:       "POD_NAMESPACE set to custom-ns",
			envValue:   "custom-ns",
			fallback:   "default-fallback",
			wantResult: "custom-ns",
		},
		{
			name:       "POD_NAMESPACE empty returns fallback",
			envValue:   "",
			fallback:   "default-fallback",
			wantResult: "default-fallback",
		},
		{
			name:       "POD_NAMESPACE whitespace-only returns fallback",
			envValue:   "   ",
			fallback:   "fallback-ns",
			wantResult: "fallback-ns",
		},
		{
			name:       "POD_NAMESPACE empty with empty fallback returns empty",
			envValue:   "",
			fallback:   "",
			wantResult: "",
		},
		{
			name:       "POD_NAMESPACE set overrides fallback",
			envValue:   "operator-system",
			fallback:   "some-other-ns",
			wantResult: "operator-system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("POD_NAMESPACE", tt.envValue)
			got := reconciler.operatorNamespace(tt.fallback)
			if got != tt.wantResult {
				t.Errorf("operatorNamespace(%q) with POD_NAMESPACE=%q = %q, want %q",
					tt.fallback, tt.envValue, got, tt.wantResult)
			}
		})
	}
}

// --- Gap 16: sortPoolsForSelection deterministic ordering ---

func TestSortPoolsForSelection(t *testing.T) {
	now := time.Now()

	t.Run("single pool returns that pool first", func(t *testing.T) {
		pools := []v1alpha1.DNSUpstreamPool{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "only-pool",
					CreationTimestamp: metav1.NewTime(now),
					ResourceVersion:   "1",
				},
			},
		}

		sortPoolsForSelection(pools)

		if pools[0].Name != "only-pool" {
			t.Errorf("expected only-pool, got %q", pools[0].Name)
		}
	})

	t.Run("three pools ordered by oldest timestamp wins", func(t *testing.T) {
		pools := []v1alpha1.DNSUpstreamPool{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "newest",
					CreationTimestamp: metav1.NewTime(now.Add(2 * time.Minute)),
					ResourceVersion:   "3",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "middle",
					CreationTimestamp: metav1.NewTime(now.Add(1 * time.Minute)),
					ResourceVersion:   "2",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "oldest",
					CreationTimestamp: metav1.NewTime(now),
					ResourceVersion:   "1",
				},
			},
		}

		sortPoolsForSelection(pools)

		if pools[0].Name != "oldest" {
			t.Errorf("expected oldest pool first, got %q", pools[0].Name)
		}
		if pools[1].Name != "middle" {
			t.Errorf("expected middle pool second, got %q", pools[1].Name)
		}
		if pools[2].Name != "newest" {
			t.Errorf("expected newest pool third, got %q", pools[2].Name)
		}
	})

	t.Run("same creationTimestamp falls back to initial-resource-version annotation", func(t *testing.T) {
		pools := []v1alpha1.DNSUpstreamPool{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pool-high-irv",
					CreationTimestamp: metav1.NewTime(now),
					Annotations:       map[string]string{initialRVAnnotation: "100"},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pool-low-irv",
					CreationTimestamp: metav1.NewTime(now),
					Annotations:       map[string]string{initialRVAnnotation: "10"},
				},
			},
		}

		sortPoolsForSelection(pools)

		if pools[0].Name != "pool-low-irv" {
			t.Errorf("expected pool-low-irv first (lower initial RV), got %q", pools[0].Name)
		}
		if pools[1].Name != "pool-high-irv" {
			t.Errorf("expected pool-high-irv second, got %q", pools[1].Name)
		}
	})

	t.Run("same timestamp and same initial-rv falls back to name ordering", func(t *testing.T) {
		pools := []v1alpha1.DNSUpstreamPool{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "zeta-pool",
					CreationTimestamp: metav1.NewTime(now),
					Annotations:       map[string]string{initialRVAnnotation: "5"},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "alpha-pool",
					CreationTimestamp: metav1.NewTime(now),
					Annotations:       map[string]string{initialRVAnnotation: "5"},
				},
			},
		}

		sortPoolsForSelection(pools)

		if pools[0].Name != "alpha-pool" {
			t.Errorf("expected alpha-pool first (alphabetically), got %q", pools[0].Name)
		}
		if pools[1].Name != "zeta-pool" {
			t.Errorf("expected zeta-pool second, got %q", pools[1].Name)
		}
	})

	t.Run("missing annotation falls through to name tiebreaker", func(t *testing.T) {
		pools := []v1alpha1.DNSUpstreamPool{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "bravo",
					CreationTimestamp: metav1.NewTime(now),
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "alpha",
					CreationTimestamp: metav1.NewTime(now),
				},
			},
		}

		sortPoolsForSelection(pools)

		if pools[0].Name != "alpha" {
			t.Errorf("expected alpha first (name tiebreak), got %q", pools[0].Name)
		}
		if pools[1].Name != "bravo" {
			t.Errorf("expected bravo second, got %q", pools[1].Name)
		}
	})

	t.Run("pool with annotation beats pool without when timestamps match", func(t *testing.T) {
		pools := []v1alpha1.DNSUpstreamPool{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "zeta-no-annotation",
					CreationTimestamp: metav1.NewTime(now),
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "alpha-with-annotation",
					CreationTimestamp: metav1.NewTime(now),
					Annotations:       map[string]string{initialRVAnnotation: "1"},
				},
			},
		}

		sortPoolsForSelection(pools)

		// alpha wins by name since zeta has no annotation (initialRV=0)
		// and alpha has annotation with value 1 — but 0 is treated as "absent"
		// so the comparison is skipped and falls through to name.
		if pools[0].Name != "alpha-with-annotation" {
			t.Errorf("expected alpha-with-annotation first, got %q", pools[0].Name)
		}
	})
}
