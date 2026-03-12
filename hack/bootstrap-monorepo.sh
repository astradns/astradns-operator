#!/usr/bin/env bash

set -euo pipefail

WITH_AGENT="${1:-}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PARENT_DIR="$(cd "${ROOT_DIR}/.." && pwd)"
OWNER="${GITHUB_REPOSITORY_OWNER:-astradns}"

clone_if_missing() {
  local repo_name="$1"
  local dest="$2"

  if [ -d "${dest}/.git" ]; then
    echo "${repo_name} already available at ${dest}"
    return
  fi

  echo "Cloning ${repo_name} into ${dest}"
  git clone --depth=1 "https://github.com/${OWNER}/${repo_name}.git" "${dest}"
}

clone_if_missing "astradns-types" "${PARENT_DIR}/astradns-types"

if [ "${WITH_AGENT}" = "--with-agent" ]; then
  clone_if_missing "astradns-agent" "${PARENT_DIR}/astradns-agent"
fi
