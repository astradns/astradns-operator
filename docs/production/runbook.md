# AstraDNS MVP Runbook

## Primary health checks

Run these commands first:

```sh
kubectl get pods -n <namespace>
kubectl get dnsupstreampools.dns.astradns.com -A
kubectl get configmap -n <namespace> <release>-agent-config -o jsonpath='{.data.config\.json}'
```

If metrics are enabled:

```sh
kubectl get servicemonitor -n <namespace>
```

## Common incidents

### 1) DNS queries fail after deploy

1. Check agent readiness (`/readyz`) and operator logs.
2. Validate CoreDNS patch job status and target forward address.
3. Confirm `DNSUpstreamPool` has `Ready=True` and no `Superseded` condition for expected active pool.
4. If needed, rollback Helm release.

### 2) CoreDNS was not patched

1. Check job:

```sh
kubectl get jobs -n <namespace> | grep coredns-patch
kubectl logs -n <namespace> job/<release>-astradns-coredns-patch
```

2. Validate values:
   - `clusterDNS.forwardExternalToAstraDNS.enabled=true`
   - `agent.network.mode=linkLocal`
3. Re-run upgrade after fixing values.

### 3) Webhook blocks legitimate changes

1. Verify if multiple pools exist in namespace.
2. Keep one active pool; delete superseded extras.
3. If emergency bypass is required, temporarily set `webhook.enabled=false` and redeploy.

## Rollback procedure

```sh
helm rollback <release> <revision> -n <namespace>
kubectl rollout status deployment/<release>-astradns-operator -n <namespace>
kubectl rollout status daemonset/<release>-astradns-agent -n <namespace>
```

Post-rollback checks:

- `kubectl get pods -n <namespace>` all Running/Ready.
- DNS resolution works from a test pod.
- `config.json` exists in agent ConfigMap.
