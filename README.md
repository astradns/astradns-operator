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
4. The rendered JSON is written to a ConfigMap in the `astradns-system` namespace.
5. The agent's config watcher detects the ConfigMap update and reloads the engine subprocess.

## Engine Selection

The operator selects the config renderer via the `ASTRADNS_ENGINE_TYPE` environment variable (default: `unbound`). Supported values: `unbound`, `coredns`, `powerdns`.

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

## License

Copyright 2026. Licensed under the Apache License, Version 2.0.
