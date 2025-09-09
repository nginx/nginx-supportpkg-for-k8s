#!/bin/bash
set -e

EXTRACTED_DIR="$1"

if [ -z "$EXTRACTED_DIR" ]; then
    echo "Usage: $0 <extracted_directory>"
    exit 1
fi

echo "Verifying contents in directory: $EXTRACTED_DIR"

# Check if extraction directory exists
if [ ! -d "$EXTRACTED_DIR" ]; then
    echo "Error: Extracted directory does not exist: $EXTRACTED_DIR"
    exit 1
fi

# Find the actual supportpkg directory (it should be nested)
SUPPORTPKG_DIR=$(find "$EXTRACTED_DIR" -maxdepth 2 -type d -name "*supportpkg*" | head -n 1)

if [ -z "$SUPPORTPKG_DIR" ]; then
    echo "Error: Could not find supportpkg directory in extracted contents"
    echo "Available directories:"
    find "$EXTRACTED_DIR" -type d | head -20
    exit 1
fi

echo "Found supportpkg directory: $SUPPORTPKG_DIR"

# List of expected files/directories based on the common and NIC job lists
EXPECTED_ITEMS=(
    "manifest.json"
    "resources/k8s-version.json"
    "resources/nodes-info.json"
    "resources/nginx-ingress/pods.json"
    "resources/nginx-ingress/events.json"
    "resources/nginx-ingress/configmaps.json"
    "resources/nginx-ingress/services.json"
    "resources/nginx-ingress/deployments.json"
    "logs/nginx-ingress"
)

# Optional items that might exist depending on what's deployed
OPTIONAL_ITEMS=(
    "exec/nginx-ingress"
    "resources/nginx-ingress/statefulsets.json"
    "resources/nginx-ingress/replicasets.json"
    "resources/nginx-ingress/leases.json"
    "resources/nginx-ingress/daemonsets.json"
    "resources/nginx-ingress/secrets.json"
    "resources/ingresses.json"
    "resources/virtualservers.json"
    "resources/virtualserverroutes.json"
)

# Check for expected items
MISSING_ITEMS=()
FOUND_ITEMS=()

for item in "${EXPECTED_ITEMS[@]}"; do
    FULL_PATH="$SUPPORTPKG_DIR/$item"
    if [ -e "$FULL_PATH" ]; then
        FOUND_ITEMS+=("$item")
        echo "✓ Found: $item"
    else
        MISSING_ITEMS+=("$item")
        echo "✗ Missing: $item"
    fi
done

# Check optional items
for item in "${OPTIONAL_ITEMS[@]}"; do
    FULL_PATH="$SUPPORTPKG_DIR/$item"
    if [ -e "$FULL_PATH" ]; then
        echo "✓ Found optional: $item"
    else
        echo "○ Optional missing: $item"
    fi
done

# Check if logs directory contains pod logs
LOGS_DIR="$SUPPORTPKG_DIR/logs/nginx-ingress"
if [ -d "$LOGS_DIR" ]; then
    LOG_COUNT=$(find "$LOGS_DIR" -name "*.txt" | wc -l)
    if [ "$LOG_COUNT" -gt 0 ]; then
        echo "✓ Found $LOG_COUNT log files in nginx-ingress namespace"
        echo "  Log files:"
        find "$LOGS_DIR" -name "*.txt" -exec basename {} \; | sed 's/^/    /'
    else
        echo "✗ No log files found in logs/nginx-ingress directory"
        MISSING_ITEMS+=("pod log files")
    fi
fi

# Check if manifest.json is valid JSON and contains expected fields
MANIFEST_FILE="$SUPPORTPKG_DIR/manifest.json"
if [ -f "$MANIFEST_FILE" ]; then
    if jq empty "$MANIFEST_FILE" 2>/dev/null; then
        echo "✓ manifest.json is valid JSON"
        PRODUCT=$(jq -r '.product // "unknown"' "$MANIFEST_FILE")
        TOTAL_JOBS=$(jq -r '.totalJobs // "unknown"' "$MANIFEST_FILE")
        FAILED_JOBS=$(jq -r '.failedJobs // "unknown"' "$MANIFEST_FILE")

        echo "  Product: $PRODUCT"
        echo "  Total jobs: $TOTAL_JOBS"
        echo "  Failed jobs: $FAILED_JOBS"

        # Verify we're testing the right product
        if [ "$PRODUCT" != "nic" ]; then
            echo "⚠ Warning: Expected product 'nic', got '$PRODUCT'"
        fi

        # Check if there are failed jobs
        if [ "$FAILED_JOBS" != "0" ] && [ "$FAILED_JOBS" != "unknown" ]; then
            echo "⚠ Warning: Some jobs failed ($FAILED_JOBS failures)"
        fi
    else
        echo "✗ manifest.json is not valid JSON"
        MISSING_ITEMS+=("valid manifest.json")
    fi
fi

# Check for kubernetes resource files content
PODS_FILE="$SUPPORTPKG_DIR/resources/nginx-ingress/pods.json"
if [ -f "$PODS_FILE" ]; then
    if jq empty "$PODS_FILE" 2>/dev/null; then
        POD_COUNT=$(jq '.items | length' "$PODS_FILE" 2>/dev/null || echo "0")
        echo "✓ Found $POD_COUNT pods in nginx-ingress namespace"
    else
        echo "✗ pods.json is not valid JSON"
        MISSING_ITEMS+=("valid pods.json")
    fi
fi

# Summary
echo ""
echo "=== VERIFICATION SUMMARY ==="
echo "Found items: ${#FOUND_ITEMS[@]}"
echo "Missing items: ${#MISSING_ITEMS[@]}"

if [ ${#MISSING_ITEMS[@]} -eq 0 ]; then
    echo "✅ All expected items found!"
    echo ""
    echo "Complete directory structure:"
    find "$SUPPORTPKG_DIR" -type f | sort | sed 's/^/  /'
    exit 0
else
    echo "❌ Some expected items are missing:"
    for item in "${MISSING_ITEMS[@]}"; do
        echo "  - $item"
    done
    echo ""
    echo "Actual directory structure:"
    find "$SUPPORTPKG_DIR" -type f | sort | sed 's/^/  /'
    exit 1
fi
