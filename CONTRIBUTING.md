# Contributing to astradns-operator

Thanks for contributing to the AstraDNS control plane.

## Pull Request Checklist

- Keep reconciliation logic idempotent and status-driven.
- Run `make test` and `make lint` locally.
- Use conventional commits (`feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `chore:`).
- Update manifests with `make manifests` when RBAC markers or CRD types change.
- Avoid broad RBAC permissions unless strictly required.

## AI/OpenCode Contributions

AI-assisted changes are welcome, but must follow repository guardrails in `AGENTS.md`.

Minimum requirements for AI-generated changes:

- No secrets, credentials, or personal data.
- Preserve import boundaries (`astradns-operator` must not import `astradns-agent`).
- Keep Kubebuilder scaffold markers intact.
- Do not manually edit generated files under `config/crd/bases` and generated RBAC.
