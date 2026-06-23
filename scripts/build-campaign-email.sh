#!/bin/bash
# Build a campaign email by inserting a phishing link into a lure template.
# Usage: ./build-campaign-email.sh <lure-template> <phishing-link> [recipient-name] [output-file]
#   recipient-name is optional (defaults to "Colleague") -- BCC mode sends identical content to all.

LURE="${1:-shared-document}"
LINK="${2}"
NAME="${3:-Colleague}"
OUTPUT="${4:-campaign-email.html}"

LURE_DIR="$(dirname "$0")/../lures"
LURE_FILE="$LURE_DIR/${LURE}.html"

if [ ! -f "$LURE_FILE" ]; then
  echo "Available lures:"
  ls "$LURE_DIR"/*.html | sed 's|.*/||;s|\.html||'
  echo ""
  echo "Usage: $0 <lure-name> <phishing-link> [recipient-name] [output-file]"
  echo "Example: $0 shared-document 'https://glnt.cc/#abc123' email.html"
  echo "Example: $0 shared-document 'https://glnt.cc/#abc123' 'Colleague' email.html"
  exit 1
fi

sed "s|{LINK}|${LINK}|g; s|{RECIPIENT_NAME}|${NAME}|g" "$LURE_FILE" > "$OUTPUT"
echo "✓ Campaign email built: $OUTPUT"
echo "  Lure: $LURE"
echo "  Link: $LINK"
echo "  Recipient: $NAME"
echo ""
echo "Copy the contents of $OUTPUT into SuperMailer's HTML editor."
