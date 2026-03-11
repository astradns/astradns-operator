#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASE_DIR="${ROOT_DIR}/config/crd/bases"
HELM_CRD_DIR="${ROOT_DIR}/deploy/helm/astradns/templates/crds"

for base_file in "${BASE_DIR}"/*.yaml; do
	base_name="$(basename "${base_file}")"
	helm_name="${base_name#*_}"
	out_file="${HELM_CRD_DIR}/${helm_name}"

	{
		echo '{{- if .Values.crds.install }}'
		echo '---'
		awk '
			{ print }
			$0 == "metadata:" {
				print "  labels:"
				print "    {{- include \"astradns.labels\" . | nindent 4 }}"
			}
		' "${base_file}"
		echo '{{- end }}'
	} >"${out_file}"
done
