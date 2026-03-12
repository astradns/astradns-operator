#!/usr/bin/env bash

set -euo pipefail

SLO_NAMESPACE="${SLO_NAMESPACE:-astradns-slo}"
SLO_RELEASE="${SLO_RELEASE:-astradns}"
SLO_ITERATIONS="${SLO_ITERATIONS:-200}"
SLO_DOMAIN="${SLO_DOMAIN:-example.com}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REPORT_DIR="${ROOT_DIR}/test/slo/reports"
mkdir -p "${REPORT_DIR}"
REPORT_FILE="${REPORT_DIR}/mvp-slo-$(date +%Y%m%d-%H%M%S).txt"

echo "AstraDNS MVP SLO validation" | tee "${REPORT_FILE}"
echo "namespace=${SLO_NAMESPACE} release=${SLO_RELEASE}" | tee -a "${REPORT_FILE}"

echo "[1/5] Measuring install time" | tee -a "${REPORT_FILE}"
start_ts=$(date +%s)
helm upgrade --install "${SLO_RELEASE}" "${ROOT_DIR}/deploy/helm/astradns" \
  --namespace "${SLO_NAMESPACE}" --create-namespace \
  -f "${ROOT_DIR}/deploy/helm/astradns/values-production.yaml" \
  --set webhook.enabled=false \
  --set clusterDNS.forwardExternalToAstraDNS.enabled=false >/dev/null
end_ts=$(date +%s)
install_seconds=$((end_ts - start_ts))
echo "install_time_seconds=${install_seconds}" | tee -a "${REPORT_FILE}"

echo "[2/5] Waiting for operator and agent readiness" | tee -a "${REPORT_FILE}"
kubectl rollout status "deployment/${SLO_RELEASE}-astradns-operator" -n "${SLO_NAMESPACE}" --timeout=180s | tee -a "${REPORT_FILE}"
kubectl rollout status "daemonset/${SLO_RELEASE}-astradns-agent" -n "${SLO_NAMESPACE}" --timeout=180s | tee -a "${REPORT_FILE}"

echo "[3/5] Running DNS latency sampling" | tee -a "${REPORT_FILE}"
node_ip=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
if [ -z "${node_ip}" ]; then
  echo "could not resolve node internal IP" | tee -a "${REPORT_FILE}"
  exit 1
fi
latency_file="${REPORT_DIR}/latency-$(date +%Y%m%d-%H%M%S).txt"
for _ in $(seq 1 "${SLO_ITERATIONS}"); do
  kubectl run "dns-slo-$RANDOM" -n "${SLO_NAMESPACE}" --rm -i --restart=Never \
    --image=busybox:1.37 --command -- sh -c "TIMEFORMAT='%R';time nslookup -port=5353 '${SLO_DOMAIN}' '${node_ip}' >/dev/null 2>&1" \
    2>>"${latency_file}" >/dev/null || true
done
sort -n "${latency_file}" > "${latency_file}.sorted"
count=$(wc -l < "${latency_file}.sorted" | tr -d ' ')
if [ "${count}" -gt 0 ]; then
  idx=$(( (count * 95 + 99) / 100 ))
  p95=$(sed -n "${idx}p" "${latency_file}.sorted")
else
  p95="n/a"
fi
echo "latency_p95_seconds=${p95}" | tee -a "${REPORT_FILE}"

echo "[4/5] Scraping cache hit ratio" | tee -a "${REPORT_FILE}"
kubectl port-forward -n "${SLO_NAMESPACE}" "service/${SLO_RELEASE}-astradns-agent-metrics" 19153:9153 >/tmp/astradns-slo-portforward.log 2>&1 &
pf_pid=$!
trap 'kill "${pf_pid}" >/dev/null 2>&1 || true' EXIT
sleep 2
metrics_output=$(curl -fsSL http://127.0.0.1:19153/metrics)
kill "${pf_pid}" >/dev/null 2>&1 || true
trap - EXIT
hits=$(printf "%s" "${metrics_output}" | awk '/^astradns_cache_hits_total /{print $2}' | tail -n 1)
misses=$(printf "%s" "${metrics_output}" | awk '/^astradns_cache_misses_total /{print $2}' | tail -n 1)
hits=${hits:-0}
misses=${misses:-0}
ratio=$(awk -v h="${hits}" -v m="${misses}" 'BEGIN { if (h+m == 0) print 0; else printf "%.4f", h/(h+m) }')
echo "cache_hits_total=${hits}" | tee -a "${REPORT_FILE}"
echo "cache_misses_total=${misses}" | tee -a "${REPORT_FILE}"
echo "cache_hit_ratio=${ratio}" | tee -a "${REPORT_FILE}"

echo "[5/5] Capturing SERVFAIL/timeout counters" | tee -a "${REPORT_FILE}"
servfail=$(printf "%s" "${metrics_output}" | awk '/^astradns_servfail_total /{print $2}' | tail -n 1)
timeouts=$(printf "%s" "${metrics_output}" | awk '/^astradns_timeout_total/{sum += $2} END {print sum+0}')
echo "servfail_total=${servfail:-0}" | tee -a "${REPORT_FILE}"
echo "timeout_total=${timeouts:-0}" | tee -a "${REPORT_FILE}"

echo "SLO report written to ${REPORT_FILE}" | tee -a "${REPORT_FILE}"
