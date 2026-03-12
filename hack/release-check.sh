#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHART_FILE="${ROOT_DIR}/deploy/helm/astradns/Chart.yaml"

if [ ! -f "${CHART_FILE}" ]; then
  echo "Chart.yaml not found at ${CHART_FILE}" >&2
  exit 1
fi

chart_version=$(awk -F': ' '$1=="version" {print $2}' "${CHART_FILE}" | tr -d '"' | head -n1)
app_version=$(awk -F': ' '$1=="appVersion" {print $2}' "${CHART_FILE}" | tr -d '"' | head -n1)

if [ -z "${chart_version}" ] || [ -z "${app_version}" ]; then
  echo "version/appVersion must be set in Chart.yaml" >&2
  exit 1
fi

semver_regex='^([0-9]+)\.([0-9]+)\.([0-9]+)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$'
if [[ ! "${chart_version}" =~ ${semver_regex} ]]; then
  echo "invalid chart version: ${chart_version}" >&2
  exit 1
fi
if [[ ! "${app_version}" =~ ${semver_regex} ]]; then
  echo "invalid appVersion: ${app_version}" >&2
  exit 1
fi

if [ -n "${RELEASE_TAG:-}" ]; then
  expected_tag="v${app_version}"
  if [ "${RELEASE_TAG}" != "${expected_tag}" ]; then
    echo "RELEASE_TAG (${RELEASE_TAG}) must match ${expected_tag}" >&2
    exit 1
  fi
fi

echo "Chart metadata looks valid: version=${chart_version} appVersion=${app_version}"

helm lint "${ROOT_DIR}/deploy/helm/astradns"
helm template check "${ROOT_DIR}/deploy/helm/astradns" >/dev/null
helm template check "${ROOT_DIR}/deploy/helm/astradns" \
  -f "${ROOT_DIR}/deploy/helm/astradns/values-production.yaml" \
  --set webhook.certManager.issuerRef.name=cluster-issuer >/dev/null

echo "Release checks passed"
