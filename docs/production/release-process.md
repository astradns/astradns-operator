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

## Release artifacts

- Helm chart package (`dist/astradns-<version>.tgz`)
- Release notes with:
  - breaking changes
  - required values updates
  - rollback command

## Rollback command

```sh
helm rollback <release> <revision> -n <namespace>
```
