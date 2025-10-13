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

# List of expected files/directories based on the common and NIC job lists
EXPECTED_ITEMS=(
    "manifest.json"
    "supportpkg.log"
    "k8s/crd.json"
    "k8s/nodes.json"
    "k8s/version.json"
)

# Check for expected items
MISSING_ITEMS=()
FOUND_ITEMS=()

for item in "${EXPECTED_ITEMS[@]}"; do
    FULL_PATH="$EXTRACTED_DIR/$item"
    if [ -e "$FULL_PATH" ]; then
        FOUND_ITEMS+=("$item")
        echo "✓ Found: $item"
    else
        MISSING_ITEMS+=("$item")
        echo "✗ Missing: $item"
    fi
done

# Check if manifest.json is valid JSON and contains expected fields
MANIFEST_FILE="$EXTRACTED_DIR/manifest.json"
if [ -f "$MANIFEST_FILE" ]; then
    if jq empty "$MANIFEST_FILE" 2>/dev/null; then
        echo "✓ manifest.json is valid JSON"
    else
        echo "✗ manifest.json is not valid JSON"
        MISSING_ITEMS+=("valid manifest.json")
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
    find "$EXTRACTED_DIR" -type f | sort | sed 's/^/  /'
    exit 0
else
    echo "❌ Some expected items are missing:"
    for item in "${MISSING_ITEMS[@]}"; do
        echo "  - $item"
    done
    echo ""
    echo "Actual directory structure:"
    find "$EXTRACTED_DIR" -type f | sort | sed 's/^/  /'
    exit 1
fi
