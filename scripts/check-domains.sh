#!/usr/bin/env bash
# Check domain availability across multiple names/TLDs via Route 53 Domains.
#
# Usage:
#   ./check-domains.sh yumaverse oio              # names, uses default TLDs
#   TLDS="com io app" ./check-domains.sh yumaverse
#   ./check-domains.sh yumaverse.com oio.io       # full domains (TLD ignored)
#
# Notes:
#   - route53domains API only lives in us-east-1.
#   - Requires AWS_PROFILE with route53domains:CheckDomainAvailability.

set -uo pipefail

PROFILE="${AWS_PROFILE:-rocket}"
REGION="us-east-1"
TLDS="${TLDS:-com io app dev co net}"

if [[ $# -eq 0 ]]; then
  echo "usage: $0 <name|domain> [name|domain ...]" >&2
  exit 1
fi

# Build the list of fully-qualified domains to check.
domains=()
for arg in "$@"; do
  if [[ "$arg" == *.* ]]; then
    domains+=("$arg")
  else
    for tld in $TLDS; do
      domains+=("${arg}.${tld}")
    done
  fi
done

printf '%-32s %s\n' "DOMAIN" "AVAILABILITY"
printf '%-32s %s\n' "------" "------------"

for domain in "${domains[@]}"; do
  status=$(AWS_PROFILE="$PROFILE" aws route53domains check-domain-availability \
    --region "$REGION" --domain-name "$domain" \
    --query 'Availability' --output text 2>/dev/null) || status="ERROR"
  printf '%-32s %s\n' "$domain" "$status"
done
