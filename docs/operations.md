# Operations runbook

Day-to-day running of the native install. Commands run on the server unless noted.

- [Services and ports](#services-and-ports)
- [Health check](#health-check)
- [Logs](#logs)
- [Backups](#backups)
- [Restoring](#restoring)
- [Rehearsing a migration](#rehearsing-a-migration)
- [Accounts and access](#accounts-and-access)
- [Troubleshooting](#troubleshooting)

## Services and ports

| Service | Unit | Listens on | Reachable from |
| --- | --- | --- | --- |
| HomeOps | `homeops` | 127.0.0.1:8090 | Internet via the tunnel only |
| PostgreSQL | `postgresql` | 127.0.0.1:5432 | Local only |
| SeaweedFS | `homeops-seaweedfs` | 127.0.0.1:8333 (+9333/8381/8889) | Local only |
| Cloudflare Tunnel | `cloudflared` | (outbound) | — |
| SSH | `ssh` | 22 | LAN + ZeroTier (firewall), keys only |
| Mail (Stalwart) | `stalwart` | 127.0.0.1:2525/4190/8480, 587, 993 | 587/993 from LAN + ZeroTier only |
| Mail bridge | `homeops-mailbridge` | 127.0.0.1:8025 | Internet via the tunnel (token required) |
| Webmail | `nginx` + `php*-fpm` | 127.0.0.1:8282 | Internet via the tunnel |

Timers: `homeops-backup.timer` (nightly), `mail-backup.timer` (03:40),
`homeguard.timer` (every 10 minutes). Check them with `systemctl list-timers`.

## Health check

```sh
systemctl --failed                                   # should list nothing
systemctl is-active homeops homeops-seaweedfs postgresql cloudflared
curl -s https://homeops.example.com/api/health      # {"status":"ok"}
sudo ufw status | head -1                            # Status: active
sudo journalctl -t homeguard --since -1d            # security alerts, if any
free -m; df -h /
```

## Logs

| What | Command |
| --- | --- |
| App (requests, scheduler, push) | `journalctl -u homeops -f` |
| Push delivery per device | `journalctl -u homeops \| grep -E "push (sent\|rejected\|subscription removed)"` |
| Tunnel | `journalctl -u cloudflared -f` |
| Storage | `journalctl -u homeops-seaweedfs` |
| Mail | `journalctl -u stalwart -u homeops-mailbridge` |
| Security watchdog | `sudo journalctl -t homeguard` |
| SSH bans | `sudo fail2ban-client status sshd` |

## Backups

Nightly, kept 30 days, in `/var/lib/homeops/backups`:

| File | Contents |
| --- | --- |
| `homeops-YYYY-MM-DD.sql.gz` | The whole database (`pg_dump --clean`) |
| `files-YYYY-MM-DD.tar.gz` | Uploaded photos, receipts and PDFs (SeaweedFS data) |
| `mail-YYYY-MM-DD.tar.gz` | Mailboxes and mail configuration (kept 14 days; Stalwart stops for a few seconds) |

```sh
sudo systemctl start homeops-backup     # take one now
ls -lt /var/lib/homeops/backups | head
```

These backups sit on the same disk. Copy them somewhere else regularly (a USB disk,
or cloud storage via rclone). A dead disk otherwise loses both the data and its backups.

## Restoring

```sh
sudo ~/homeops/infra/native/restore.sh /var/lib/homeops/backups/homeops-2026-10-23.sql.gz \
                                       /var/lib/homeops/backups/files-2026-10-23.tar.gz
```

- It asks you to type `restore`, stops the app, replaces **all** data, fixes table
  ownership and starts the app again. Restoring an older backup is fine: newer
  migrations are applied on start.
- With a files archive, the current files are kept in
  `/var/lib/homeops/<dir>.before-restore` until you delete them.
- To check a backup without touching live data, restore it into a scratch database:

```sh
sudo -u postgres createdb -O homeops homeops_restoretest
sudo RESTORE_DB=homeops_restoretest ~/homeops/infra/native/restore.sh /var/lib/homeops/backups/homeops-….sql.gz
sudo -u postgres psql -d homeops_restoretest -c "select count(*) from users"
sudo -u postgres dropdb homeops_restoretest
```

Mail: `sudo systemctl stop stalwart && sudo tar -xzf mail-….tar.gz -C /` restores
`/var/lib/stalwart` and `/etc/stalwart`. Then `sudo systemctl start stalwart`.

## Rehearsing a migration

Before deploying a release with a big migration, run the new binary against a copy
of the live database on another port:

```sh
sudo -u postgres createdb -O homeops homeops_migtest
sudo -u postgres sh -c "pg_dump homeops | psql -q -d homeops_migtest"
URL=$(sudo grep ^DATABASE_URL= /etc/homeops/homeops.env | cut -d= -f2- | sed 's#/homeops?#/homeops_migtest?#')
DATABASE_URL="$URL" HOMEOPS_ADDR=127.0.0.1:8099 HOMEOPS_PUBLIC_URL=https://homeops.example.com \
  HOMEOPS_DATA_DIR=/tmp/migtest timeout 30 ~/homeops/.local/homeops &
sleep 5; curl -s http://127.0.0.1:8099/api/health      # migrated copy answers
sudo -u postgres psql -d homeops_migtest -c "select max(version) from schema_migrations"
sudo -u postgres dropdb homeops_migtest
```

## Accounts and access

| Task | How |
| --- | --- |
| Add someone | Settings → Members → invite by email (owner/admin) |
| New household | Administration → New household (platform administrator) |
| Someone forgot their password and email isn't set up | `sudo ~/homeops/infra/native/reset-password.sh <username or email>`: sets a new password (typed hidden) and signs them out everywhere. With mail set up ([mail.md](mail.md)), they can use *Forgot password?* themselves |
| Make someone platform administrator | Administration → People → *Make admin* |
| Sign out a lost phone | That person: Settings → Sign-in & security → sessions → sign out |
| Change the server's sudo password | `passwd` (SSH is key-only; see [security.md](security.md)) |

## Troubleshooting

**Notifications don't arrive on a device.** Open **Settings → Notifications** on that
device and check **Your devices**:

- If the device isn't listed, enable notifications again *on that device*.
- On iPhone it must be the Home Screen app opened from Safari; Chrome on iOS can't
  receive web push.
- If it's listed with an error, the reason from Apple or Google is shown, and is also
  in `journalctl -u homeops | grep push`. A device the push service reports as gone
  (404/410) is removed automatically and has to be enabled again.

**Site shows Cloudflare error 1033 or 502.** The tunnel or app is down: check
`systemctl status cloudflared homeops`. A 502 with a healthy app usually means the
tunnel's public hostname points at the wrong port.

**"Can't reach HomeOps" in the app, but the site works.** It's an old cached build.
Reload, or accept the "new version" prompt.

**Uploads fail.** Check `systemctl status homeops-seaweedfs` and the disk space (`df -h`).

**A migration failed on start.** The app logs the failing migration and exits; nothing
half-applied stays, because each migration runs in a transaction. Restore the
previous binary (`/opt/homeops/homeops` from the old bundle) or fix forward.
