# astradns-operator

AstraDNS Operator is the control-plane component of AstraDNS. It watches custom resources that describe DNS configuration intent, renders engine-specific configuration, and writes the result to a ConfigMap consumed by the agent DaemonSet.

## Custom Resource Definitions

All CRDs belong to the API group `dns.astradns.com/v1alpha1`.

| CRD | Description |
|---|---|
| `DNSUpstreamPool` | Defines upstream DNS resolvers with health checking and selection policy |
| `DNSCacheProfile` | Configures DNS cache behavior (TTL bounds, prefetch, negative caching) |
| `ExternalDNSPolicy` | Controls external DNS zone delegation and filtering rules |

## How It Works

```
CRD changes --> Reconciler --> EngineConfig assembly
                                    |
                                    v
                            ConfigRenderer (engine-specific)
                                    |
                                    v
                            Validate + render config
                                    |
                                    v
                            Write JSON to ConfigMap
                                    |
                                    v
                            Agent detects change and reloads
```

1. The operator watches `DNSUpstreamPool`, `DNSCacheProfile`, and `ExternalDNSPolicy` resources.
2. On change, the reconciler assembles an `EngineConfig` from the current state of all relevant CRs.
3. The appropriate `ConfigRenderer` (selected by `ASTRADNS_ENGINE_TYPE`) validates and renders the config.
4. The rendered JSON is written to the `astradns-agent-config` ConfigMap in the operator namespace.
5. The agent's config watcher detects the ConfigMap update and reloads the engine subprocess.

## Runtime Contract With Agent

- ConfigMap name: `astradns-agent-config`
- ConfigMap key: `config.json`
- Operator namespace is passed to controllers via `POD_NAMESPACE` (set in `config/manager/manager.yaml`).
- Agent should be deployed in the same namespace so it can mount this ConfigMap directly.

## Engine Selection

The operator selects the config renderer via the `ASTRADNS_ENGINE_TYPE` environment variable (default: `unbound`). Supported values: `unbound`, `coredns`, `powerdns`, `bind`.

When deployed with Helm, this value is taken from `agent.engineType` and propagated to both operator and agent.

## Helm Production Profile

The chart includes a production profile at `deploy/helm/astradns/values-production.yaml` with:

- Link-local data path (`agent.network.mode=linkLocal`, `169.254.20.11`)
- Optional cluster DNS integration job (`clusterDNS.forwardExternalToAstraDNS.enabled=true`)
- PodDisruptionBudgets + NetworkPolicies + PriorityClass defaults
- Agent service account token automount disabled by default
- ServiceMonitor and Grafana dashboard ConfigMap enabled
- Validating webhook enabled (cert-manager issuer still required)

Install example:

```sh
helm upgrade --install astradns deploy/helm/astradns \
  -n astradns-system --create-namespace \
  -f deploy/helm/astradns/values-production.yaml \
  --set webhook.certManager.issuerRef.name=<cluster-issuer-name>
```

## CoreDNS Integration

With `clusterDNS.forwardExternalToAstraDNS.enabled=true`, Helm runs a post-install/post-upgrade job that patches the CoreDNS ConfigMap forward target to the AstraDNS link-local listener (`169.254.20.11:5353` by default).

`clusterDNS.provider` is currently validated to `coredns` when this integration is enabled.

Helm validates integration inputs early: `namespace`, `configMapName`, and optional `rolloutDeployment` must be valid Kubernetes resource names, `forwardTarget` must be `host:port` (or `[ipv6]:port`), and `kubectlImagePullPolicy` must be one of `Always`, `IfNotPresent`, or `Never`.

To avoid misconfiguration, cluster DNS integration requires `agent.network.mode=linkLocal`.

The integration toggle remains explicit because some clusters patch CoreDNS out-of-band (GitOps/platform controllers), and Helm should not overwrite cluster DNS unless requested.

## Engine Image Policy

The chart pins images to AstraDNS official artifacts for the selected chart version:

- Operator: `ghcr.io/astradns/astradns-operator:v<appVersion>`
- Agent: `ghcr.io/astradns/astradns-agent:v<appVersion>-<engine>`

Users only select the engine flavor via `agent.engineType`.

## Prerequisites

- Go 1.24.6+
- Docker 17.03+
- kubectl v1.11.3+
- Access to a Kubernetes v1.11.3+ cluster

## Installation

```sh
# Install CRDs into the cluster
make install

# Deploy the operator
make deploy IMG=<registry>/astradns-operator:<tag>

# Deploy the agent DaemonSet in the same namespace
kubectl apply -f ../astradns-agent/config/daemonset.yaml

# Apply sample CRs
kubectl apply -k config/samples/
```

## Uninstall

```sh
# Remove CRs
kubectl delete -k config/samples/

# Remove CRDs
make uninstall

# Remove the operator
make undeploy
```

## Development

```sh
# Generate CRD manifests from Go types
make manifests

# Run unit tests (uses envtest for controller-runtime integration)
make test

# Run static analysis
make vet
```

## Release Gates

Production release workflows and SLO validation assets live in:

- `docs/production/go-live-checklist.md`
- `docs/production/coredns-integration.md`
- `docs/production/runbook.md`
- `docs/production/slo-validation.md`
- `docs/production/release-process.md`

## Contribution Policy

- Human and AI contributions: `CONTRIBUTING.md`
- OpenCode-specific guardrails: `OPENCODE_RULES.md`
- Repository-level AI constraints: `AGENTS.md`

## License

Copyright 2026. Licensed under the Apache License, Version 2.0.
