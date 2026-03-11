package controllers

import (
	"testing"
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
