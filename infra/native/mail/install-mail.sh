#!/usr/bin/env bash
# Self-hosted mail for one domain on the HomeOps server, without opening port 25.
#
#   sudo ./infra/native/mail/install-mail.sh [--domain sayan.fit]
#
#   Inbound:  Cloudflare Email Routing → Email Worker → https://mailin.<domain> (tunnel)
#             → mailbridge (127.0.0.1:8025) → Stalwart SMTP (127.0.0.1:2525)
#   Outbound: Stalwart → Brevo SMTP relay (set BREVO_LOGIN/BREVO_SMTP_KEY, then re-run)
#   Webmail:  Roundcube on 127.0.0.1:8282 → https://mail.<domain> (tunnel)
#   Clients:  IMAPS 993 / submission 587 on LAN and ZeroTier only (firewall)
#   Admin UI: http://127.0.0.1:8480/admin (ssh -L 8480:127.0.0.1:8480 to reach it)
#
# Secrets are generated once into /etc/homeops/mail.env (root only) and never printed.
# Re-running is safe: it converges the configuration and keeps passwords.
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo "Run with sudo"; exit 1; }
HERE=$(cd "$(dirname "$0")" && pwd)
REPO=$(cd "$HERE/../../.." && pwd)
DOMAIN=sayan.fit
[[ "${1:-}" == --domain ]] && DOMAIN=$2
HOST="mail.$DOMAIN"

STALWART_VERSION=0.16.25
STALWART_SHA256=8ba9cc0ea2795121df5c1264af41e82353c5a3d6860161c4ab70817906d912d3
CLI_VERSION=1.0.13
CLI_SHA256=1b8509b767edd1a17693e092b518c41610ec4b724f4e62c461bd28cd40c669f7
ROUNDCUBE_VERSION=1.7.4
ROUNDCUBE_SHA256=2c6c878f0093f1bf7fb6086781d2dd9269d652c016b86939c157c5f1729139a2
RECOVERY_PORT=18080   # 8080 belongs to qBittorrent
SECRETS=/etc/homeops/mail.env

say() { echo "▸ $*"; }
rand() { head -c 48 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c "${1:-24}"; }

fetch() { # url sha256 dest — prefers a copy shipped in the bundle's .local/
  local url=$1 sum=$2 dest=$3 local_copy="$REPO/.local/$(basename "$1")"
  if [[ -f $local_copy ]]; then cp "$local_copy" "$dest"; else
    curl -fsSL --retry 3 --speed-limit 20000 --speed-time 60 -o "$dest" "$url"; fi
  echo "$sum  $dest" | sha256sum -c --quiet || { echo "✖ checksum mismatch for $url"; exit 1; }
}

# ---------- secrets ----------
install -d -m 0750 /etc/homeops
touch "$SECRETS"; chmod 600 "$SECRETS"
# shellcheck disable=SC1090
. "$SECRETS"
first_install=false
[[ -n ${PASS_me:-} ]] || first_install=true
: "${STALWART_ADMIN_PASS:=$(rand 24)}" "${PASS_me:=$(rand 20)}" "${PASS_no_reply:=$(rand 32)}"
: "${MAILBRIDGE_TOKEN:=$(rand 48)}" "${BREVO_LOGIN:=}" "${BREVO_SMTP_KEY:=}"
cat >"$SECRETS" <<EOF
# HomeOps mail secrets — root only. Set BREVO_LOGIN / BREVO_SMTP_KEY and re-run install-mail.sh.
STALWART_ADMIN_PASS=$STALWART_ADMIN_PASS
PASS_me=$PASS_me
PASS_no_reply=$PASS_no_reply
MAILBRIDGE_TOKEN=$MAILBRIDGE_TOKEN
BREVO_LOGIN=$BREVO_LOGIN
BREVO_SMTP_KEY=$BREVO_SMTP_KEY
EOF

# ---------- Stalwart ----------
say "Stalwart $STALWART_VERSION"
id -u stalwart >/dev/null 2>&1 || useradd stalwart -s /usr/sbin/nologin -M -r -U
tmp=$(mktemp -d)
# Whatever happens, never leave the server stuck in setup mode with mail down.
cleanup() {
  rm -rf "$tmp"
  if systemctl is-active -q stalwart-setup 2>/dev/null; then systemctl stop stalwart-setup; fi
  systemctl reset-failed stalwart-setup 2>/dev/null || true
  [[ -f /etc/stalwart/config.json ]] && ! systemctl is-active -q stalwart && systemctl start stalwart
  return 0
}
trap cleanup EXIT
if ! /usr/local/bin/stalwart --version 2>/dev/null | grep -q "$STALWART_VERSION"; then
  fetch "https://github.com/stalwartlabs/stalwart/releases/download/v$STALWART_VERSION/stalwart-x86_64-unknown-linux-gnu.tar.gz" "$STALWART_SHA256" "$tmp/s.tgz"
  tar -xzf "$tmp/s.tgz" -C "$tmp" stalwart
  install -m 0755 "$tmp/stalwart" /usr/local/bin/stalwart
fi
if ! /usr/local/bin/stalwart-cli --version 2>/dev/null | grep -q "$CLI_VERSION"; then
  fetch "https://github.com/stalwartlabs/cli/releases/download/v$CLI_VERSION/stalwart-cli-x86_64-unknown-linux-gnu.tar.xz" "$CLI_SHA256" "$tmp/c.txz"
  tar -xJf "$tmp/c.txz" -C "$tmp"
  install -m 0755 "$tmp"/stalwart-cli-*/stalwart-cli /usr/local/bin/stalwart-cli
fi
install -d -m 0750 -o stalwart -g stalwart /etc/stalwart /var/lib/stalwart
# Stalwart reads the relay key from its environment, never from its database.
install -m 0640 -o root -g stalwart /dev/null /etc/stalwart/stalwart.env
printf 'BREVO_SMTP_KEY=%s\nSTALWART_PUBLIC_URL=https://%s\n' "$BREVO_SMTP_KEY" "$HOST" >/etc/stalwart/stalwart.env
install -m 0644 "$HERE/stalwart.service" /etc/systemd/system/stalwart.service
systemctl daemon-reload

export STALWART_USER STALWART_PASSWORD STALWART_URL
cli() { /usr/local/bin/stalwart-cli --no-color "$@"; }
wait_http() { for _ in $(seq 1 120); do curl -fs -o /dev/null "$1" && return 0; sleep 0.5; done; echo "✖ $1 not answering; journalctl -u stalwart"; exit 1; }
# One-off runs in bootstrap/recovery mode on loopback, as the stalwart user.
run_recovery() {
  systemctl stop stalwart 2>/dev/null || true
  systemctl stop stalwart-setup 2>/dev/null || true
  systemctl reset-failed stalwart-setup 2>/dev/null || true
  systemd-run --quiet --unit=stalwart-setup --uid=stalwart --gid=stalwart -p EnvironmentFile=/etc/stalwart/stalwart.env \
    -E STALWART_RECOVERY_MODE="$1" -E STALWART_RECOVERY_MODE_PORT=$RECOVERY_PORT -E STALWART_RECOVERY_ADMIN="admin:$STALWART_ADMIN_PASS" \
    -E STALWART_HOSTNAME="$HOST" /usr/local/bin/stalwart --config=/etc/stalwart/config.json
  STALWART_URL=http://127.0.0.1:$RECOVERY_PORT STALWART_USER=admin STALWART_PASSWORD=$STALWART_ADMIN_PASS
  wait_http "$STALWART_URL/"
}
stop_recovery() {
  systemctl stop stalwart-setup 2>/dev/null || true
  for _ in $(seq 1 40); do systemctl is-active -q stalwart-setup || break; sleep 0.25; done
  systemctl reset-failed stalwart-setup 2>/dev/null || true
}

if [[ ! -f /etc/stalwart/config.json ]]; then
  say "First start: setting up $HOST for $DOMAIN"
  run_recovery 0
  cli update Bootstrap --json "$(python3 -c '
import json,sys; h,d=sys.argv[1:]
print(json.dumps({"serverHostname":h,"defaultDomain":d,"requestTlsCertificate":False,"generateDkimKeys":True,
 "dataStore":{"@type":"RocksDb","path":"/var/lib/stalwart/"},
 "tracer":{"@type":"Stdout","ansi":False,"level":"info","enable":True,"multiline":False,"lossy":False,"events":{},"eventsPolicy":"exclude"}}))' "$HOST" "$DOMAIN")" >/dev/null
  for _ in $(seq 1 40); do [[ -f /etc/stalwart/config.json ]] && break; sleep 0.25; done
  stop_recovery
fi

say "Applying mail configuration"
run_recovery 1
MAIL_DOMAIN=$DOMAIN MAILBOXES="me:Sayan" ALIASES="postmaster:me abuse:me dmarc:me hostmaster:me" \
  PASS_me=$PASS_me PASS_no_reply=$PASS_no_reply PASS_admin=$STALWART_ADMIN_PASS BREVO_LOGIN=$BREVO_LOGIN \
  python3 "$HERE/plan.py" >"$tmp/plan.ndjson"
cli apply --file "$tmp/plan.ndjson" | tail -3
# Webmail and local services sign in from 127.0.0.1; never auto-ban loopback.
cli apply --file <(printf '%s\n' '{"@type":"upsert","object":"AllowedIp","matchOn":["address"],"value":{"lo4":{"address":"127.0.0.0/8","reason":"local webmail, bridge and apps"},"lo6":{"address":"::1","reason":"local webmail, bridge and apps"}}}') | tail -1
stop_recovery
systemctl enable --now stalwart >/dev/null 2>&1
systemctl restart stalwart
for _ in $(seq 1 60); do (echo >/dev/tcp/127.0.0.1/2525) 2>/dev/null && break; sleep 0.5; done

# ---------- mailbridge ----------
say "mailbridge"
install -m 0755 "$REPO/.local/mailbridge" /opt/homeops/mailbridge
install -m 0600 /dev/null /etc/homeops/mailbridge.env
printf 'MAILBRIDGE_TOKEN=%s\nMAILBRIDGE_HELO=%s\nMAILBRIDGE_SMTP=127.0.0.1:2525\nMAILBRIDGE_ADDR=127.0.0.1:8025\n' "$MAILBRIDGE_TOKEN" "$HOST" >/etc/homeops/mailbridge.env
install -m 0644 "$HERE/homeops-mailbridge.service" /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now homeops-mailbridge >/dev/null 2>&1
systemctl restart homeops-mailbridge

# ---------- Roundcube (upstream release; Ubuntu's package lags security fixes) ----------
say "Roundcube $ROUNDCUBE_VERSION webmail"
if dpkg -s roundcube-core >/dev/null 2>&1; then
  DEBIAN_FRONTEND=noninteractive apt-get purge -y -qq roundcube roundcube-core roundcube-sqlite3 roundcube-plugins >/dev/null 2>&1 || true
fi
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends nginx php-fpm php-intl php-mbstring \
  php-xml php-sqlite3 php-curl php-zip php-gd >/dev/null
RC=/opt/roundcube/roundcubemail-$ROUNDCUBE_VERSION
if [[ ! -d $RC ]]; then
  fetch "https://github.com/roundcube/roundcubemail/releases/download/$ROUNDCUBE_VERSION/roundcubemail-$ROUNDCUBE_VERSION-complete.tar.gz" "$ROUNDCUBE_SHA256" "$tmp/rc.tgz"
  install -d /opt/roundcube
  tar -xzf "$tmp/rc.tgz" -C /opt/roundcube
  chown -R root:root "$RC"
  rm -rf "$RC/installer" "$RC/public_html/installer.php"
fi
install -d -m 0750 -o www-data -g www-data /var/lib/roundcube-homeops /var/lib/roundcube-homeops/temp
install -d -m 0750 -o root -g www-data /etc/roundcube-homeops
[[ -s /etc/roundcube-homeops/des.key ]] || { rand 24 >/etc/roundcube-homeops/des.key; }
chown root:www-data /etc/roundcube-homeops/des.key; chmod 640 /etc/roundcube-homeops/des.key
sed "s/__DOMAIN__/$DOMAIN/g" "$HERE/roundcube.inc.php" >/etc/roundcube-homeops/homeops.inc.php
chmod 640 /etc/roundcube-homeops/homeops.inc.php; chgrp www-data /etc/roundcube-homeops/homeops.inc.php
printf "<?php\n\$config = [];\ninclude '/etc/roundcube-homeops/homeops.inc.php';\n" >"$RC/config/config.inc.php"
ln -sfn "$RC" /opt/roundcube/current
# The SQLite driver creates the schema on first connect; updatedb then applies upgrades.
(cd "$RC" && sudo -u www-data php bin/updatedb.sh --package=roundcube --dir=SQL >/dev/null)
PHP_FPM=$(systemctl list-unit-files 'php*-fpm.service' --no-legend | awk '{print $1}' | sort -V | tail -1)
systemctl enable --now "$PHP_FPM" >/dev/null 2>&1
PHP_SOCK=/run/php/${PHP_FPM%.service}.sock
sed "s#__PHP_SOCK__#$PHP_SOCK#" "$HERE/nginx-roundcube.conf" >/etc/nginx/sites-available/roundcube
ln -sf /etc/nginx/sites-available/roundcube /etc/nginx/sites-enabled/roundcube
rm -f /etc/nginx/sites-enabled/default   # nginx must not listen publicly
nginx -t -q
systemctl enable --now nginx >/dev/null 2>&1
systemctl reload nginx

# ---------- HomeOps & homeguard send through the local server ----------
ENV=/etc/homeops/homeops.env
# Only once mail can actually leave the server; until then HomeOps shows invite links to copy.
if [[ -n $BREVO_LOGIN ]]; then
say "HomeOps email"
for kv in "HOMEOPS_SMTP_HOST=127.0.0.1" "HOMEOPS_SMTP_PORT=587" "HOMEOPS_SMTP_USER=no-reply@$DOMAIN" \
          "HOMEOPS_SMTP_PASSWORD=$PASS_no_reply" "HOMEOPS_MAIL_FROM=HomeOps <no-reply@$DOMAIN>"; do
  k=${kv%%=*}; grep -v "^$k=" "$ENV" >"$ENV.tmp"; echo "$kv" >>"$ENV.tmp"; mv "$ENV.tmp" "$ENV"
done
chmod 640 "$ENV"; chgrp homeops "$ENV" 2>/dev/null || true
systemctl restart homeops
fi
if [[ -f /etc/homeguard.conf ]]; then
  sed -i -e "s|^ALERT_TO=.*|ALERT_TO=\"\${ALERT_TO_OVERRIDE:-me@$DOMAIN}\"|" -e "s|^MAIL_FROM=.*|MAIL_FROM=\"no-reply@$DOMAIN\"|" \
    -e "s|^SMTP_URL=.*|SMTP_URL=\"smtp://127.0.0.1:587\"|" -e "s|^SMTP_USER=.*|SMTP_USER=\"no-reply@$DOMAIN\"|" \
    -e "s|^SMTP_PASS=.*|SMTP_PASS=\"$PASS_no_reply\"|" /etc/homeguard.conf
  /usr/local/sbin/homeguard --rebaseline >/dev/null
fi

# ---------- nightly mail backup ----------
install -m 0644 "$HERE/mail-backup.service" "$HERE/mail-backup.timer" /etc/systemd/system/
systemctl daemon-reload
systemctl enable --now mail-backup.timer >/dev/null 2>&1

echo
echo "✔ Mail installed for $DOMAIN"
[[ -n $BREVO_LOGIN ]] && echo "  Outbound: via Brevo ($BREVO_LOGIN)" || echo "  Outbound: NOT configured yet — add BREVO_LOGIN/BREVO_SMTP_KEY to $SECRETS and re-run"
$first_install && echo "  Mailbox me@$DOMAIN — password: sudo grep PASS_me $SECRETS (change it in webmail)"
echo "  Worker secret BRIDGE_TOKEN: sudo grep MAILBRIDGE_TOKEN $SECRETS"
echo "  DNS records to publish:     sudo $HERE/dns-records.sh"
