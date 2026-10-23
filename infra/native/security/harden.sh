#!/usr/bin/env bash
# Baseline hardening for the home server. Idempotent; safe to re-run.
#
#   sudo ./infra/native/security/harden.sh [--trusted "192.168.0.0/24 10.167.183.0/24"]
#
# - Firewall (ufw): nothing inbound except from the trusted networks (LAN, ZeroTier),
#   plus the torrent and ZeroTier ports. Cloudflare Tunnel is outbound-only, so the
#   public sites keep working.
# - SSH: keys only, no root, few attempts; fail2ban bans brute-forcers.
# - Kernel: common sysctl hardening; /dev/shm mounted noexec (a favourite miner drop spot).
# - homeguard: a 10-minute watchdog for crypto miners and persistence (see homeguard.sh).
#
# The firewall comes up with a 5-minute automatic rollback; run
#   sudo systemctl stop ufw-rollback.timer
# from a NEW ssh session once you've confirmed you can still get in.
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo "Run with sudo"; exit 1; }
HERE=$(cd "$(dirname "$0")" && pwd)

TRUSTED="192.168.0.0/24 10.167.183.0/24"
[[ "${1:-}" == --trusted ]] && TRUSTED=$2
TORRENT_PORT=29475   # qBittorrent listening port (Settings → Connection)

if [[ -n "${SUDO_USER:-}" ]] && ! grep -qs . "$(getent passwd "$SUDO_USER" | cut -d: -f6)/.ssh/authorized_keys"; then
  echo "✖ $SUDO_USER has no SSH key in ~/.ssh/authorized_keys; refusing to disable password login."
  exit 1
fi

echo "▸ Packages"
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ufw fail2ban >/dev/null

echo "▸ SSH: keys only"
cat >/etc/ssh/sshd_config.d/10-homeops-hardening.conf <<'EOF'
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin no
PermitEmptyPasswords no
MaxAuthTries 3
LoginGraceTime 30
X11Forwarding no
AllowAgentForwarding no
EOF
sshd -t
systemctl reload ssh

echo "▸ fail2ban"
cat >/etc/fail2ban/jail.d/homeops.local <<'EOF'
[DEFAULT]
backend = systemd
bantime = 1h
bantime.increment = true
findtime = 10m
maxretry = 5

[sshd]
enabled = true
EOF
systemctl enable --now fail2ban >/dev/null 2>&1
systemctl restart fail2ban

echo "▸ Kernel settings"
cat >/etc/sysctl.d/90-homeops-hardening.conf <<'EOF'
# Ignore spoofed and redirected packets
net.ipv4.conf.all.rp_filter = 1
net.ipv4.conf.default.rp_filter = 1
net.ipv4.conf.all.accept_redirects = 0
net.ipv4.conf.default.accept_redirects = 0
net.ipv6.conf.all.accept_redirects = 0
net.ipv6.conf.default.accept_redirects = 0
net.ipv4.conf.all.send_redirects = 0
net.ipv4.conf.all.accept_source_route = 0
net.ipv6.conf.all.accept_source_route = 0
net.ipv4.tcp_syncookies = 1
net.ipv4.icmp_echo_ignore_broadcasts = 1
net.ipv4.conf.all.log_martians = 1
# Make kernel exploits harder
kernel.kptr_restrict = 2
kernel.dmesg_restrict = 1
kernel.yama.ptrace_scope = 1
kernel.unprivileged_bpf_disabled = 1
net.core.bpf_jit_harden = 2
fs.protected_symlinks = 1
fs.protected_hardlinks = 1
fs.suid_dumpable = 0
EOF
sysctl -q --system

echo "▸ /dev/shm noexec"
if ! grep -qE '^\S+\s+/dev/shm\s' /etc/fstab; then
  echo 'tmpfs /dev/shm tmpfs defaults,nosuid,nodev,noexec 0 0' >>/etc/fstab
fi
mount -o remount,nosuid,nodev,noexec /dev/shm

echo "▸ Automatic security updates"
cat >/etc/apt/apt.conf.d/20auto-upgrades <<'EOF'
APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";
APT::Periodic::AutocleanInterval "7";
EOF

echo "▸ homeguard watchdog"
install -m 0755 "$HERE/homeguard.sh" /usr/local/sbin/homeguard
install -m 0644 "$HERE/homeguard.service" "$HERE/homeguard.timer" /etc/systemd/system/
install -d -m 0700 /var/lib/homeguard
[[ -f /etc/homeguard.conf ]] || install -m 0600 "$HERE/homeguard.conf.example" /etc/homeguard.conf
systemctl daemon-reload
/usr/local/sbin/homeguard --rebaseline
systemctl enable --now homeguard.timer >/dev/null 2>&1

echo "▸ Firewall"
ufw --force reset >/dev/null
ufw default deny incoming >/dev/null
ufw default allow outgoing >/dev/null
for net in $TRUSTED; do
  ufw allow from "$net" to any port 22 proto tcp comment 'ssh' >/dev/null
  ufw allow from "$net" to any port 8096 proto tcp comment 'emby' >/dev/null
  ufw allow from "$net" to any port 8080 proto tcp comment 'qbittorrent webui' >/dev/null
  ufw allow from "$net" to any port 993,587 proto tcp comment 'mail clients' >/dev/null
  ufw allow from "$net" to any port 1900,7359 proto udp comment 'emby/dlna discovery' >/dev/null
  ufw allow from "$net" to any port 6771 proto udp comment 'torrent local peer discovery' >/dev/null
done
ufw allow "$TORRENT_PORT" comment 'torrent peers' >/dev/null
ufw allow 9993/udp comment 'zerotier' >/dev/null
# The rollback turns the firewall off again unless cancelled, so a mistake can't lock you out.
systemctl stop ufw-rollback.timer 2>/dev/null || true
systemd-run --quiet --unit=ufw-rollback --on-active=300 /usr/sbin/ufw disable
ufw --force enable >/dev/null
ufw status numbered
echo
echo "✔ Hardened. Open a NEW ssh session to confirm access, then run:"
echo "    sudo systemctl stop ufw-rollback.timer"
echo "  (otherwise the firewall switches itself off in 5 minutes)"
