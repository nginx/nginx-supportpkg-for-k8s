#!/usr/bin/env bash
set -euo pipefail

# Exit codes:
#  0 - success
#  3 - missing dependency (jq)
#  4 - manifest.json not found
#  5 - manifest.json exists but is not a regular file
#  6 - manifest.json is not valid JSON
#  7 - missing required JSON keys
#  8 - missing files in the supportpkg
#  9 - missing files in the manifest

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
  # Test presence: use getpath(split(".")) and check that it exists and is not null
  if ! jq -e --arg p "$p" 'getpath($p | split(".")) != null' "$manifest" >/dev/null 2>&1; then
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

# Check that there is a 1:1 correspondence between manifest and tarball content

find "${dir}" -type f -mindepth 1 | cut -c "$(( ${#dir} + 1 ))"- > find-output.txt
jq -r '.commands[].output' "${dir}"/manifest.json > manifest-output.txt
printf "/manifest.json\n/supportpkg.log" >> manifest-output.txt
if grep -vxFf find-output.txt manifest-output.txt >/dev/null; then
  echo "ERROR: Manifest contains files not present in the supportpkg:"
  grep -vxFf find-output.txt manifest-output.txt
  exit 8
fi
if grep -vxFf manifest-output.txt find-output.txt >/dev/null; then
  echo "ERROR: Supportpkg contains files not present in the manifest:"
  grep -vxFf manifest-output.txt find-output.txt
  exit 9
fi

# All good if we reached this point

echo "OK: $manifest is valid and consistent with the supportpkg contents"
exit 0
