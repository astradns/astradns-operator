# Release Process

## Versioning

- Use semantic version tags: `vX.Y.Z`.
- Chart version and appVersion live in `deploy/helm/astradns/Chart.yaml`.

## Pre-release checks

1. `make test`
2. `make lint`
3. `make test-e2e`
4. `make test-integration`
5. `make test-slo`
6. `make release-check`

## Packaging

```sh
helm package deploy/helm/astradns --destination dist
```

The release workflow also performs chart packaging on tags.

## Helm distribution

Tagged releases publish the chart as OCI to GHCR:

- `oci://ghcr.io/astradns/helm-charts/astradns:<tag>`

Example install:

```sh
helm install astradns oci://ghcr.io/astradns/helm-charts/astradns --version 0.2.0 -n astradns-system --create-namespace
```

## Upgrade strategy (Canary and Blue/Green)

Use one of the following rollout profiles for production upgrades.

### Canary rollout (default for patch/minor)

1. Deploy chart with the new version to a canary node pool or limited namespace scope.
2. Keep production traffic split with a small canary percentage (5-10%) for at least one SLO window.
3. Validate DNS latency, error rate, and upstream health against baseline.
4. If healthy, promote the same chart version to remaining nodes in controlled batches.
5. If unhealthy, execute rollback immediately using `helm rollback`.

### Blue/Green rollout (recommended for high-risk changes)

1. Deploy a parallel release (`astradns-green`) with isolated values and labels.
2. Keep `astradns-blue` serving all traffic while running readiness and synthetic DNS checks on green.
3. Switch traffic source (CoreDNS forward target / service selector) from blue to green in one controlled step.
4. Observe one full validation window before decommissioning blue.
5. On any regression, flip traffic back to blue and investigate before retry.

## Release artifacts

- Helm chart package (`dist/astradns-<version>.tgz`)
- CycloneDX SBOMs for operator source and Helm chart (`dist/*-sbom.cdx.json`)
- Release notes with:
  - breaking changes
  - required values updates
  - rollback command

## Rollback command

```sh
helm rollback <release> <revision> -n <namespace>
```
