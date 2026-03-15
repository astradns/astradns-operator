# CoreDNS Integration Guide

This guide enables automatic CoreDNS forwarding to AstraDNS through Helm.

## Topology-aware forwarding

CoreDNS integration behaves differently based on `agent.topology.profile`.

| Profile | Forward target source |
|---|---|
| `node-local` | `clusterDNS.forwardExternalToAstraDNS.forwardTarget` (chart default: `169.254.20.11:5353`) |
| `central` + `agent.dnsService.clusterIP` set | fixed Service ClusterIP + `agent.dnsService.port` |
| `central` + `agent.dnsService.clusterIP` empty | runtime service discovery (`kubectl get service ...`) |

When `central` mode uses runtime discovery, the patch job reads the DNS Service `clusterIP` at execution time and writes that value into the CoreDNS `forward` stanza.

## Required values

### node-local profile

```yaml
agent:
  topology:
    profile: node-local
  network:
    mode: linkLocal
    linkLocalIP: 169.254.20.11 # chart default

clusterDNS:
  forwardExternalToAstraDNS:
    enabled: true
    namespace: kube-system
    configMapName: coredns
    rolloutDeployment: coredns
    forwardTarget: 169.254.20.11:5353 # keep aligned with linkLocalIP
```

`169.254.20.11` is a default, not a fixed contract. If your environment uses a different link-local IP, keep `forwardTarget` aligned with `agent.network.linkLocalIP`.

### central profile with fixed clusterIP

```yaml
agent:
  topology:
    profile: central
  deployment:
    replicas: 3
  dnsService:
    type: ClusterIP
    clusterIP: 10.96.0.53
    port: 53

clusterDNS:
  forwardExternalToAstraDNS:
    enabled: true
    namespace: kube-system
    configMapName: coredns
    rolloutDeployment: coredns
```

### central profile with runtime service discovery

```yaml
agent:
  topology:
    profile: central
  deployment:
    replicas: 3
  dnsService:
    type: ClusterIP
    clusterIP: "" # auto-allocated by Kubernetes
    port: 53

clusterDNS:
  forwardExternalToAstraDNS:
    enabled: true
    namespace: kube-system
    configMapName: coredns
    rolloutDeployment: coredns
```

## Guardrails

- `profile=node-local` requires `agent.network.mode=linkLocal` when CoreDNS patching is enabled.
- `profile=central` rejects `agent.network.mode=linkLocal` at template render time.
- Invalid `agent.dnsService.clusterIP` values fail chart rendering.

## What Helm does

1. Creates RBAC + ServiceAccount for a patch job.
2. Runs a post-install/post-upgrade job.
3. Rewrites CoreDNS forward target in `Corefile` according to topology profile.
4. Stores backup in `Corefile.astradns.backup`.
5. Restarts CoreDNS deployment (if configured).

## Verification

```sh
kubectl -n kube-system get configmap coredns -o jsonpath='{.data.Corefile}'
kubectl -n kube-system get configmap coredns -o jsonpath='{.data.Corefile\.astradns\.backup}'
```

In central mode, verify the service address that should be present in CoreDNS:

```sh
kubectl -n <astradns-namespace> get service <release>-astradns-agent-dns -o jsonpath='{.spec.clusterIP}:{.spec.ports[0].port}'
```

## Rollback

Restore backup manually if needed:

```sh
kubectl -n kube-system patch configmap coredns --type merge --patch-file <(cat <<'EOF'
data:
  Corefile: |
    # paste the previous Corefile content from Corefile.astradns.backup
EOF
)
```
