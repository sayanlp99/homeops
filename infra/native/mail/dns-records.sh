#!/usr/bin/env bash
# Prints the DNS records to add in Cloudflare for mail on this server.
#   sudo ./infra/native/mail/dns-records.sh [--domain sayan.fit]
# MX and SPF for receiving are created by Cloudflare Email Routing itself; this lists
# Stalwart's DKIM keys (so mail it signs verifies) and the SPF/DMARC values to use.
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo "Run with sudo"; exit 1; }
DOMAIN=sayan.fit
[[ "${1:-}" == --domain ]] && DOMAIN=$2
. /etc/homeops/mail.env
export STALWART_URL=http://127.0.0.1:8480 STALWART_USER="admin@$DOMAIN" STALWART_PASSWORD="$STALWART_ADMIN_PASS"
id=$(stalwart-cli --no-color query Domain --json 2>/dev/null | python3 -c "
import json,sys; d=sys.argv[1]
for line in sys.stdin:  # one JSON object per line
    r = json.loads(line) if line.strip() else {}
    if r.get('name') == d: print(r['id'])" "$DOMAIN")
zone=$(stalwart-cli --no-color get Domain "$id" --fields dnsZoneFile --json | python3 -c 'import json,sys; print(json.load(sys.stdin)["dnsZoneFile"])')
echo "== Stalwart DKIM keys (add each as a TXT record)"
grep -i "_domainkey" <<<"$zone" || echo "(none yet; restart stalwart and re-run)"
echo
echo "== SPF (TXT on $DOMAIN) — merge with what Email Routing created:"
echo "   v=spf1 include:_spf.mx.cloudflare.net include:spf.brevo.com ~all"
echo "== DMARC (TXT on _dmarc.$DOMAIN):"
echo "   v=DMARC1; p=quarantine; adkim=r; aspf=r; rua=mailto:dmarc@$DOMAIN"
echo
echo "(Full zone Stalwart suggests, for reference — don't add its MX/autoconfig records:"
echo "$zone" | sed 's/^/   /'
echo ")"
