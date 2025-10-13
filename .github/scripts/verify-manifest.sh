#!/usr/bin/env bash
set -euo pipefail

# Exit codes:
#  0 - success
#  2 - usage / bad args
#  3 - missing dependency (jq)
#  4 - manifest.json not found
#  5 - manifest.json exists but is not a regular file
#  6 - manifest.json is not valid JSON
#  7 - missing required JSON keys

if [[ ${#} -ne 1 ]]; then
  usage
fi

dir="$1"
manifest="$dir/manifest.json"

if ! command -v jq >/dev/null 2>&1; then
  echo "ERROR: jq is required but not found in PATH" >&2
  exit 3
fi

if [[ ! -e "$manifest" ]]; then
  echo "ERROR: manifest.json not found in directory: $dir" >&2
  exit 4
fi

if [[ ! -f "$manifest" ]]; then
  echo "ERROR: $manifest exists but is not a regular file" >&2
  exit 5
fi

# Validate JSON
if ! jq empty "$manifest" >/dev/null 2>&1; then
  echo "ERROR: $manifest is not valid JSON" >&2
  # show where jq fails (best-effort)
  jq . "$manifest" 2>/dev/null || true
  exit 6
fi

required=(
  "version"
  "ts.start"
  "ts.stop"
  "package_type"
  "root_dir"
  "commands"
  "platform_info.platform_type"
  "platform_info.hostname"
  "platform_info.serial_number"
  "product_info.product"
  "product_info.version"
  "product_info.build"
)

missing=()
for p in "${required[@]}"; do
  # split dot-separated path into array elements and build a jq array literal
  IFS='.' read -r -a parts <<< "$p"
  # build JSON array like ["a","b"]
  jq_array="["
  first=1
  for part in "${parts[@]}"; do
    if [[ $first -eq 1 ]]; then
      jq_array+="\"${part}\""
      first=0
    else
      jq_array+=",\"${part}\""
    fi
  done
  jq_array+="]"

  # Test presence: use getpath(array) and check that it exists and is not null
  if ! jq -e "getpath($jq_array) != null" "$manifest" >/dev/null 2>&1; then
    missing+=("$p")
  fi
done

if [[ ${#missing[@]} -ne 0 ]]; then
  echo "ERROR: manifest.json is missing required keys:" >&2
  for m in "${missing[@]}"; do
    echo "  - $m" >&2
  done
  exit 7
fi

echo "OK: $manifest is valid and contains all required keys"
exit 0
