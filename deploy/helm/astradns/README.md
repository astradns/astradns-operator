# AstraDNS Helm Chart

The AstraDNS chart deploys:

- the AstraDNS Operator (control plane);
- the AstraDNS Agent (data plane);
- optional CoreDNS forwarding patch job;
- optional observability assets (ServiceMonitor and Grafana dashboard ConfigMap).

## Project links

- Documentation site: https://docs.astradns.com/
- Artifact Hub package: https://artifacthub.io/packages/helm/astradns/astradns

## What users configure

The chart pins official images by chart `appVersion`.
Users choose the DNS engine flavor only:

- `agent.engineType=unbound|coredns|powerdns|bind`

Image references are resolved automatically:

- operator: `ghcr.io/astradns/astradns-operator:v<appVersion>`
- agent: `ghcr.io/astradns/astradns-agent:v<appVersion>-<engine>`

## Install (OCI)

```sh
helm upgrade --install astradns oci://ghcr.io/astradns/helm-charts/astradns \
  --version <chart-version> \
  -n astradns-system --create-namespace \
  --set agent.engineType=unbound
```

## Recommended production baseline (node-local + linkLocal)

```sh
helm upgrade --install astradns oci://ghcr.io/astradns/helm-charts/astradns \
  --version <chart-version> \
  -n astradns-system --create-namespace \
  --set agent.engineType=unbound \
  --set agent.network.mode=linkLocal \
  --set clusterDNS.forwardExternalToAstraDNS.enabled=true
```

If you use cert-manager for the validating webhook:

```sh
helm upgrade --install astradns oci://ghcr.io/astradns/helm-charts/astradns \
  --version <chart-version> \
  -n astradns-system --create-namespace \
  --set agent.network.mode=linkLocal \
  --set clusterDNS.forwardExternalToAstraDNS.enabled=true \
  --set webhook.enabled=true \
  --set webhook.certManager.issuerRef.name=<cluster-issuer-name>
```

## Topology profiles

| Profile | Agent workload | DNS path |
|---|---|---|
| `node-local` (default) | DaemonSet | link-local or hostPort per node |
| `central` | Deployment + Service | ClusterIP service fronting agent replicas |

Example for central profile:

```sh
helm upgrade --install astradns oci://ghcr.io/astradns/helm-charts/astradns \
  --version <chart-version> \
  -n astradns-system --create-namespace \
  --set agent.topology.profile=central \
  --set agent.deployment.replicas=3 \
  --set clusterDNS.forwardExternalToAstraDNS.enabled=true
```

## Link-local default note

For `node-local` + `agent.network.mode=linkLocal`:

- `agent.network.linkLocalIP` default is `169.254.20.11`
- `clusterDNS.forwardExternalToAstraDNS.forwardTarget` default is `169.254.20.11:5353`

`169.254.20.11` is a chart default, not a fixed universal value.
If you customize `agent.network.linkLocalIP`, keep `clusterDNS.forwardExternalToAstraDNS.forwardTarget` aligned as `<linkLocalIP>:5353`.

## Schema and signatures

- This chart includes a values schema (`values.schema.json`) for typed values validation and Artifact Hub Values Schema UI.
- Tagged releases publish chart provenance (`.prov`) so consumers can verify integrity with Helm provenance checks.

## Key values

| Key | Default | Purpose |
|---|---|---|
| `agent.engineType` | `unbound` | Selects engine flavor |
| `agent.topology.profile` | `node-local` | Chooses DaemonSet vs Deployment+Service |
| `agent.network.mode` | `hostPort` | Agent DNS listener mode |
| `agent.network.linkLocalIP` | `169.254.20.11` | Link-local listener IP (node-local) |
| `clusterDNS.forwardExternalToAstraDNS.enabled` | `false` | Enables CoreDNS patch job |
| `clusterDNS.forwardExternalToAstraDNS.forwardTarget` | `169.254.20.11:5353` | Node-local CoreDNS target |
| `webhook.enabled` | `false` | Enables pool uniqueness validating webhook |
| `serviceMonitor.enabled` | `false` | Enables Prometheus Operator ServiceMonitor |

## Post-install checks

```sh
kubectl get pods -n astradns-system
kubectl -n kube-system get configmap coredns -o jsonpath='{.data.Corefile}'
kubectl -n astradns-system get jobs | grep coredns-patch
kubectl run dns-test --rm -it --restart=Never --image=busybox:1.37 -- nslookup example.com
```

## Upgrade and rollback

```sh
helm upgrade astradns oci://ghcr.io/astradns/helm-charts/astradns --version <chart-version> -n astradns-system
helm history astradns -n astradns-system
helm rollback astradns <revision> -n astradns-system
```

## Additional docs

- [Operator README](https://github.com/astradns/astradns-operator/blob/main/README.md)
- [CoreDNS integration guide](https://github.com/astradns/astradns-operator/blob/main/docs/production/coredns-integration.md)
- [Production runbook](https://github.com/astradns/astradns-operator/blob/main/docs/production/runbook.md)
