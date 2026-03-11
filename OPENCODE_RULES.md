# OpenCode Rules -- astradns-operator

These rules define how AI agents may contribute to this repository.

1. Run `make test` and `make lint` before opening a PR.
2. Keep reconciliation idempotent and status-condition driven.
3. Do not add dependencies without explicit maintainer approval.
4. Do not manually edit generated CRD/RBAC files.
5. Preserve Kubebuilder scaffold markers.
6. Never commit secrets, credentials, tokens, or personal data.
7. Follow `AGENTS.md` for repository-specific constraints.
