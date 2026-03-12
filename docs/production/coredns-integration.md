# CoreDNS Integration Guide

This guide enables automatic CoreDNS forwarding to AstraDNS through Helm.

## Required values

```yaml
agent:
  network:
    mode: linkLocal
    linkLocalIP: 169.254.20.11

clusterDNS:
  forwardExternalToAstraDNS:
    enabled: true
    namespace: kube-system
    configMapName: coredns
    rolloutDeployment: coredns
    forwardTarget: 169.254.20.11:5353
```

## What Helm does

1. Creates RBAC + ServiceAccount for a patch job.
2. Runs a post-install/post-upgrade job.
3. Rewrites CoreDNS forward target in `Corefile`.
4. Stores backup in `Corefile.astradns.backup`.
5. Restarts CoreDNS deployment (if configured).

## Verification

```sh
kubectl -n kube-system get configmap coredns -o jsonpath='{.data.Corefile}'
kubectl -n kube-system get configmap coredns -o jsonpath='{.data.Corefile\.astradns\.backup}'
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
