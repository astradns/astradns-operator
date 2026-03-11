# AI Agent Guidelines -- astradns-operator

This document governs AI-assisted contributions to the `astradns-operator` repository. AI agents (Claude, Copilot, and others) must follow these guidelines alongside standard project conventions.

## Principles

AI contributions are held to the same quality standards as human contributions. There are no exceptions. Every change must be reviewable, testable, and justified.

## Rules

1. **All AI-generated code must pass existing tests and linters.** Run `make test` and `make lint` before proposing any change.
2. **Do not introduce new dependencies without explicit approval.** Propose dependency additions in the PR description with a justification.
3. **Do not modify API contracts without discussion.** CRD types are defined in `astradns-types`. If changes are needed there, open a separate discussion or PR against that repository first.
4. **Do not commit secrets, credentials, or PII.** No tokens, passwords, API keys, or personal data in code, comments, or test fixtures.
5. **Follow conventional commit format.** Use prefixes such as `feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `chore:`.
6. **Respect import boundaries.** The operator imports from `astradns-types` only. It must never import from `astradns-agent`. The operator and agent are independent consumers of the shared types module.

## Repo-Specific Context

This is the **control-plane component** of AstraDNS, a Kubernetes operator built with controller-runtime and scaffolded by Kubebuilder. It contains:

- **Controllers** (`controllers/`) -- reconciliation logic for the AstraDNS custom resources:
  - `DNSCacheProfile` -- cache tuning per namespace or cluster.
  - `DNSUpstreamPool` -- upstream resolver pool management.
  - `ExternalDNSPolicy` -- external DNS access policies.
- **CRD manifests** (`config/crd/bases/`) -- generated from types in `astradns-types`. Do not edit manually.
- **RBAC manifests** (`config/rbac/`) -- generated from kubebuilder markers. Do not edit manually.
- **Entry point** (`cmd/main.go`) -- operator manager bootstrap.

### Controller-Runtime Patterns

Follow these patterns strictly:

- **Idempotent reconciliation.** The `Reconcile` method must be safe to call multiple times with the same input.
- **Always update status conditions.** Use `metav1.Condition` for status reporting. Every reconciliation must set conditions reflecting the current state.
- **Re-fetch before updates.** Call `r.Get()` to get a fresh copy before `r.Update()` or `r.Status().Update()` to avoid optimistic concurrency conflicts.
- **Owner references.** Set `controllerutil.SetControllerReference` on created resources to enable garbage collection.
- **Watch secondary resources.** Use `.Owns()` or `.Watches()` in the controller setup, not polling via `RequeueAfter` alone.
- **Finalizers.** Use finalizers for cleanup of external resources. Always remove the finalizer after cleanup succeeds.

### Generated Files -- Do Not Edit

- `config/crd/bases/*.yaml` -- regenerate with `make manifests`
- `config/rbac/role.yaml` -- regenerate with `make manifests`
- `**/zz_generated.*.go` -- regenerate with `make generate`
- `PROJECT` -- managed by Kubebuilder CLI

### Scaffold Markers

Do not delete `// +kubebuilder:scaffold:*` comments. The Kubebuilder CLI uses these markers for code generation.

## Code Style

- **Language:** Go
- **Follow existing patterns** in the codebase and controller-runtime conventions.
- **Structured logging:** Use `log.FromContext(ctx)` (controller-runtime's logr integration). Follow Kubernetes logging conventions: capitalize first word, no trailing period, past tense for completed actions.
- **Error handling:** Wrap errors with context. Return errors to the reconciler framework so it can handle requeueing.
- **RBAC markers:** Keep `+kubebuilder:rbac` markers accurate and minimal. Do not request permissions broader than necessary.

## Testing Expectations

- **Unit tests** use envtest (real Kubernetes API server and etcd, no mocking). The test suite is in `controllers/suite_test.go`.
- Tests use **Ginkgo and Gomega** (BDD style). Follow the existing test patterns.
- Each controller must have tests covering:
  - Successful reconciliation (resource created, status updated).
  - Error handling (missing dependencies, invalid input).
  - Status condition transitions.
  - Idempotency (reconciling an already-reconciled resource is a no-op).
- Run `make test` to execute the full test suite (includes manifest generation, code generation, formatting, vetting, and envtest setup).
- Run `make lint` for linting via golangci-lint.
- E2E tests (`make test-e2e`) require an isolated Kind cluster. Do not run them against shared or production clusters.

## Build and Deployment

- `make build` produces the manager binary at `bin/manager`.
- `make docker-build IMG=<image>` builds the container image.
- `make deploy IMG=<image>` deploys the operator to the current Kubernetes context.
- `make manifests` regenerates CRDs and RBAC from markers and types. Run this after any change to types or RBAC markers.
