# AstraDNS MVP Go-Live Checklist

This checklist is the release gate for promoting AstraDNS MVP to production.

## 1) Helm CoreDNS integration

- [ ] `clusterDNS.forwardExternalToAstraDNS.enabled=true` validated in staging.
- [ ] CoreDNS ConfigMap patch job completed successfully.
- [ ] CoreDNS rollout restart completed and DNS remained available.

## 2) Node-local data path mode

- [ ] `agent.network.mode=linkLocal` enabled.
- [ ] Agent binds to `169.254.20.11:5353` on all nodes.
- [ ] CoreDNS forward target matches the configured link-local endpoint.

## 3) Engine image strategy

- [ ] Selected engine (`agent.engineType`) has a runnable image.
- [ ] If using custom images, `agent.engineImages.<engine>` is set and pulled successfully.
- [ ] Engine switch tested in non-prod (`unbound`/`coredns`/`powerdns`).

## 4) Webhook enforcement profile

- [ ] `webhook.enabled=true` in production values.
- [ ] cert-manager issuer configured (`webhook.certManager.issuerRef.name`).
- [ ] Uniqueness policy validated (second `DNSUpstreamPool` create is denied).

## 4.1) Operator service account hardening

- [ ] `operator.serviceAccount.automountServiceAccountToken=true` set explicitly for in-cluster API auth.
- [ ] Token mount rationale reviewed and accepted for the target environment.

## 5) CI release gate

- [ ] Unit tests pass (`make test`).
- [ ] Lint passes (`make lint`).
- [ ] E2E suite passes (`make test-e2e`).
- [ ] Integration suite passes (`make test-integration`).

## 6) Observability completeness

- [ ] Prometheus scraping enabled (`serviceMonitor.enabled=true`).
- [ ] Dashboard ConfigMap enabled (`grafana.dashboards.enabled=true`).
- [ ] Dashboard contains operator + agent metrics and Loki top-domains panel.

## 7) SLO validation

- [ ] Install time <= 5 minutes.
- [ ] p95 DNS latency <= baseline.
- [ ] Cache hit ratio > 30% under representative load.
- [ ] Recovery from upstream failure < 30s.
- [ ] No AstraDNS-induced resolution failures in validation window.

## 8) Runbook readiness

- [ ] On-call runbook published and reviewed.
- [ ] Rollback runbook tested in staging.

## 9) Documentation alignment

- [ ] README and chart values reflect actual behavior.
- [ ] Known MVP limitations are explicitly documented.

## 10) Release process

- [ ] Release version selected and tagged.
- [ ] Helm chart packaged from tagged commit.
- [ ] Release notes include upgrade and rollback instructions.
