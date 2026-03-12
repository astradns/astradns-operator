# MVP SLO Validation

This document defines the repeatable procedure to validate MVP SLOs before release.

## Target SLOs

1. Install time < 5 minutes.
2. p95 external DNS latency <= baseline.
3. Cache hit ratio > 30% under representative load.
4. Recovery from upstream failure < 30s.
5. Zero AstraDNS-induced DNS failures during validation window.

## Runner

Use:

```sh
make test-slo
```

The script is located at `test/slo/validate-mvp.sh`.

## Inputs

Set these environment variables as needed:

- `SLO_NAMESPACE` (default: `astradns-slo`)
- `SLO_RELEASE` (default: `astradns`)
- `SLO_ITERATIONS` (default: `200`)
- `SLO_DOMAIN` (default: `example.com`)

## Output

The command writes a timestamped report under `test/slo/reports/`.
