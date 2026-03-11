package main

import "testing"

func TestIsPoolUniquenessWebhookEnabled(t *testing.T) {
	t.Setenv(enablePoolUniquenessWebhookEnv, "true")
	if !isPoolUniquenessWebhookEnabled() {
		t.Fatal("expected true for value 'true'")
	}

	t.Setenv(enablePoolUniquenessWebhookEnv, "1")
	if !isPoolUniquenessWebhookEnabled() {
		t.Fatal("expected true for value '1'")
	}

	t.Setenv(enablePoolUniquenessWebhookEnv, "false")
	if isPoolUniquenessWebhookEnabled() {
		t.Fatal("expected false for value 'false'")
	}

	t.Setenv(enablePoolUniquenessWebhookEnv, "garbage")
	if isPoolUniquenessWebhookEnabled() {
		t.Fatal("expected false for invalid boolean value")
	}

	t.Setenv(enablePoolUniquenessWebhookEnv, "")
	if isPoolUniquenessWebhookEnabled() {
		t.Fatal("expected false for empty value")
	}
}
