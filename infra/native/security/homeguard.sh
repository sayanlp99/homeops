#!/usr/bin/env bash
# homeguard: lightweight intrusion and crypto-miner watchdog, run every 10 minutes.
#
# Alerts (to the journal, and by email when /etc/homeguard.conf has SMTP settings) on:
#   - processes that look like miners (names, stratum pool URLs, deleted executables)
#   - any process outside the allowlist burning >70% of a core for two runs in a row
#   - executables sitting in /tmp, /var/tmp or /dev/shm
#   - changes since the baseline: listening ports, login-capable or uid-0 users,
#     SSH authorized_keys, sudoers, cron jobs, systemd units, setuid binaries
#
#   homeguard               check (what the timer runs)
#   homeguard --rebaseline  accept the current state as normal (after a deliberate change)
#   homeguard --test        send a test alert
set -uo pipefail
STATE=/var/lib/homeguard
CONF=/etc/homeguard.conf
mkdir -p "$STATE"
ALERT_TO="" SMTP_URL="" SMTP_USER="" SMTP_PASS="" MAIL_FROM=""
# Processes allowed to be busy, as user:command (e.g. Emby transcoding).
CPU_ALLOW="emby:EmbyServer emby:ffmpeg qbtuser:qbittorrent-nox postgres:postgres homeops:homeops homeops:weed root:unattended-upgr root:apt root:dpkg root:apt-get _apt:http _apt:gpgv root:fwupd root:snapd stalwart:stalwart www-data:php-fpm8.5"
[[ -f $CONF ]] && . "$CONF"

alerts=()
alert() { alerts+=("$1"); logger -t homeguard -p auth.alert "$1"; }

# ---- miners ----
miner_re='xmrig|xmr-stak|minerd|cpuminer|kdevtmpfsi|kinsing|nanominer|t-rex|lolminer|phoenixminer|nbminer|srbminer|teamredminer|gminer|ethminer|cryptonight|randomx|stratum\+(tcp|ssl)|pool\.(minexmr|supportxmr|hashvault)|nicehash|moneroocean|c3pool'
while read -r pid user args; do
  alert "Possible crypto miner: pid $pid user $user: ${args:0:200}"
done < <(ps -eo pid=,user=,args= | grep -Ei "$miner_re" | grep -v -e homeguard -e grep)

# Running programs whose file was deleted: a classic way to hide a dropped binary.
for exe in /proc/[0-9]*/exe; do
  t=$(readlink "$exe" 2>/dev/null) || continue
  if [[ $t == *" (deleted)" && $t != /usr/* && $t != /opt/* && $t != /memfd:* ]]; then
    pid=${exe#/proc/}; pid=${pid%/exe}
    alert "Process $pid runs a deleted file: $t ($(ps -o user=,args= -p "$pid" | cut -c1-150))"
  fi
done

# ---- sustained CPU ----
# Compare each process's CPU time with the previous run; >70% of a core twice in a row alerts.
now=$(date +%s)
prev_t=$(cat "$STATE/cpu.time" 2>/dev/null || echo 0)
elapsed=$((now - prev_t))
declare -A prev hot_prev
while read -r pid secs; do prev[$pid]=$secs; done < <(cat "$STATE/cpu.prev" 2>/dev/null)
while read -r pid; do hot_prev[$pid]=1; done < <(cat "$STATE/cpu.hot" 2>/dev/null)
: >"$STATE/cpu.new"; : >"$STATE/cpu.hotnew"
tick=$(getconf CLK_TCK)
while read -r pid user comm; do
  stat=$(cat "/proc/$pid/stat" 2>/dev/null) || continue
  rest=${stat##*) }
  read -r -a f <<<"$rest"
  secs=$(((f[11] + f[12]) / tick))
  echo "$pid $secs" >>"$STATE/cpu.new"
  [[ -n ${prev[$pid]:-} && $elapsed -gt 60 && $elapsed -lt 1800 ]] || continue
  used=$((secs - prev[$pid]))
  if ((used * 100 > elapsed * 70)); then
    case " $CPU_ALLOW " in *" $user:$comm "*) continue ;; esac
    echo "$pid" >>"$STATE/cpu.hotnew"
    [[ -n ${hot_prev[$pid]:-} ]] && alert "Sustained high CPU: pid $pid ($user/$comm) used $((used * 100 / elapsed))% of a core: $(ps -o args= -p "$pid" | cut -c1-150)"
  fi
done < <(ps -eo pid=,user=,comm=)
mv "$STATE/cpu.new" "$STATE/cpu.prev"; mv "$STATE/cpu.hotnew" "$STATE/cpu.hot"; echo "$now" >"$STATE/cpu.time"

# ---- dropped executables ----
while read -r f; do
  alert "Executable file in a temp directory: $f ($(stat -c '%U %y' "$f" | cut -c1-40))"
done < <(find /tmp /var/tmp /dev/shm -xdev -type f -perm /111 -mmin -1440 2>/dev/null | grep -v -e '/systemd-private-' | head -20)

# ---- persistence & exposure, compared with the baseline ----
snapshot() {
  echo "## listening"
  ss -H -ltnu 2>/dev/null | awk '{print $1, $5}' | sed -E 's/%[a-z0-9]+//; s/:[0-9]{5}$/:EPHEMERAL/' | sort -u
  echo "## users"
  awk -F: '$3 == 0 || $7 !~ /(nologin|false|sync)$/ {print $1, $3, $7}' /etc/passwd
  echo "## authorized_keys"
  for f in /root/.ssh/authorized_keys /home/*/.ssh/authorized_keys /var/lib/*/.ssh/authorized_keys; do
    [[ -f $f ]] && echo "$f $(sha256sum <"$f" | cut -c1-16)"
  done
  echo "## sudoers"
  cat /etc/sudoers /etc/sudoers.d/* 2>/dev/null | grep -vE '^\s*(#|$)' | sha256sum | cut -c1-16
  getent group sudo
  echo "## cron"
  ls -1 /etc/cron.d /etc/cron.daily /etc/cron.hourly /var/spool/cron/crontabs 2>/dev/null | grep -v -e '^$' -e ':$'
  cat /var/spool/cron/crontabs/* /etc/crontab /etc/cron.d/* 2>/dev/null | grep -vE '^\s*(#|$)' | sha256sum | cut -c1-16
  echo "## units"
  ls /etc/systemd/system /usr/local/lib/systemd/system 2>/dev/null | grep -E '\.(service|timer|socket|path)$'
  echo "## setuid"
  find / -xdev \( -perm -4000 -o -perm -2000 \) -type f 2>/dev/null | grep -v -E '^/(snap|var/lib/snapd)/' | sort
}

if [[ "${1:-}" == --rebaseline ]]; then
  snapshot >"$STATE/baseline"
  echo "homeguard: baseline saved"
  exit 0
fi
if [[ -f $STATE/baseline ]]; then
  if ! d=$(diff <(cat "$STATE/baseline") <(snapshot)); then
    changes=$(grep -E '^[<>]' <<<"$d" | head -30)
    # Report each change once: the new state becomes the baseline after alerting.
    alert "System changes since last check (< removed, > added):
$changes
If these were deliberate, nothing to do."
    snapshot >"$STATE/baseline"
  fi
else
  snapshot >"$STATE/baseline"
fi

[[ "${1:-}" == --test ]] && alert "Test alert from homeguard on $(hostname)"

# ---- deliver ----
if ((${#alerts[@]})) && [[ -n $ALERT_TO && -n $SMTP_URL ]]; then
  body=$(printf '%s\n\n' "${alerts[@]}")
  {
    printf 'From: %s\r\nTo: %s\r\nSubject: [homeguard] %s alert(s) on %s\r\nDate: %s\r\n\r\n' \
      "${MAIL_FROM:-homeguard@$(hostname)}" "$ALERT_TO" "${#alerts[@]}" "$(hostname)" "$(date -R)"
    printf '%s\n' "$body"
    printf '\nReview with: sudo journalctl -t homeguard\n'
  } | curl -s --max-time 30 --url "$SMTP_URL" --ssl-reqd --insecure \
      ${SMTP_USER:+--user "$SMTP_USER:$SMTP_PASS"} \
      --mail-from "${MAIL_FROM:-homeguard@localhost}" --mail-rcpt "$ALERT_TO" --upload-file - \
    || logger -t homeguard -p auth.err "could not email alerts"
fi
exit 0
