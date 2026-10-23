# HomeOps

A private household app for two: weekly sprints, chores, meal planning and budget.
Runs on your home server and is published through a Cloudflare Tunnel, so no ports are opened on your router.

**Phase 1 (done):** sign-in with password + passkeys, partner invite, installable PWA, push notifications, live sync, offline mode, Kanban board, sprint planning, recurring chores, notification inbox.
**Phase 2 (done):** weekly meal plan (breakfast/lunch/snacks/dinner, several dishes per slot), recipe library with photos and import-from-link, rule-based suggestions and auto-fill, grocery list built from the plan, pantry with low-stock and expiry tracking, meal & prep notifications.
**Phase 3 (done):** monthly budget envelopes (shared + private personal), 3-tap expense entry with receipt photos, bank statement import with learned categorisation, grocery invoice import (Zepto, Blinkit), bills with reminders, budget alerts.
**Phase 4 (done):** sprint retros (private cards, reveal, votes, action items become next-sprint tasks), burndown, velocity and fairness charts, daily streaks and kudos.

## Stack

| Layer | Choice |
| --- | --- |
| API | Go (chi, pgx), single ~15 MB binary in a distroless image |
| DB | PostgreSQL 17 |
| Files | SeaweedFS (S3 API) for recipe photos and, later, receipts — internal only, served through the app behind sign-in |
| Web | React 19 + Vite + TanStack Query + Tailwind; Workbox service worker |
| Realtime | Server-Sent Events (works through Cloudflare Tunnel) |
| Push | Standard Web Push with VAPID keys generated on first start |

## Try it on this PC (no Docker, no Cloudflare)

```sh
./run-local.sh          # or: make local      (add --reset to start from scratch)
```

It builds the app, starts an embedded Postgres (data in `backend/.devdata`) and serves everything on **http://localhost:8080**. The first visit creates the admin account; invite your partner from Settings → Household. Requires Go 1.27+ (the script also looks in `~/.local/go`) and Node 24+.

Phones on the same Wi-Fi can open `http://<this-pc-ip>:8080` (the script prints it). Browsers only allow passkeys, push notifications and offline mode on https or localhost, so those are unavailable that way; everything else works. For the full experience on phones, deploy with the tunnel below.

## Deploy on the home server without Docker (recommended)

Everything runs as plain systemd services: Postgres (distro package), SeaweedFS, the HomeOps binary and cloudflared. The server needs no Go or Node.

```sh
# on your PC
make bundle                                   # → .local/homeops-bundle.tar.gz
scp .local/homeops-bundle.tar.gz server:/tmp/
# on the server
tar xzf /tmp/homeops-bundle.tar.gz -C ~ && cd ~/homeops
nano .env        # HOMEOPS_PUBLIC_URL, HOMEOPS_VAPID_EMAIL, POSTGRES_PASSWORD, S3_ACCESS_KEY, S3_SECRET_KEY, TUNNEL_TOKEN
sudo ./infra/native/install.sh --prebuilt --port 8090     # re-run any time; data is kept
```

The app listens on `127.0.0.1:<port>` only; point the tunnel's public hostname at `http://localhost:<port>`. Config is in `/etc/homeops/homeops.env`; logs via `journalctl -u homeops -f`; nightly backups in `/var/lib/homeops/backups` (`sudo systemctl start homeops-backup` runs one now). To update, build a new bundle, unpack it over `~/homeops` (keep `.env`) and run `sudo ./infra/native/update.sh`. Use `--storage disk` to skip SeaweedFS and keep uploads in `/var/lib/homeops/files`.

### Hardening

`sudo ./infra/native/security/harden.sh` sets a firewall that only accepts the LAN and ZeroTier (the tunnel is outbound-only, so the public sites keep working). It also makes SSH key-only and adds fail2ban, kernel hardening and a noexec `/dev/shm`. Last, it installs **homeguard**, a 10-minute watchdog for crypto miners, sustained CPU hogs, dropped executables and new users, keys, cron jobs, units or listening ports. Alerts go to the journal (`sudo journalctl -t homeguard`) and to email (`/etc/homeguard.conf`). After a deliberate system change, run `sudo homeguard --rebaseline`. The firewall comes up with a 5-minute auto-rollback; confirm access from a new SSH session, then run `sudo systemctl stop ufw-rollback.timer`.

### Mail (`you@yourdomain`, webmail, invite emails)

`sudo ./infra/native/mail/install-mail.sh --domain example.com` installs Stalwart (mailboxes, IMAP, SMTP), Roundcube webmail and a small bridge. Port 25 is never opened, and the home IP stays private:

- **Receiving:** Cloudflare Email Routing → Email Worker (`infra/native/mail/email-worker.js`) → `https://mailin.<domain>` through the tunnel → bridge → Stalwart. If the server is down, the worker forwards the message to a fallback address.
- **Sending:** a residential IP can't deliver mail directly, so Stalwart relays everything through Brevo. Put `BREVO_LOGIN` and `BREVO_SMTP_KEY` in `/etc/homeops/mail.env` and re-run the script.
- **Reading:** webmail at `https://mail.<domain>` (tunnel → `127.0.0.1:8282`). Phone and desktop mail apps can use IMAPS 993 and submission 587 on the LAN or ZeroTier.
- **HomeOps** sends invites and password resets as `no-reply@<domain>` (the `HOMEOPS_SMTP_*` settings are written for you).
- **Admin:** `ssh -L 8480:127.0.0.1:8480 server`, then open http://localhost:8480/admin as `admin@<domain>`. The password is `STALWART_ADMIN_PASS` in `mail.env`.
- **DNS:** run `sudo ./infra/native/mail/dns-records.sh` for the DKIM, SPF and DMARC records. The script is safe to re-run, and mail is backed up nightly by `mail-backup.timer`.

## Deploy with Docker

### 1. Install Docker (once)

```sh
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker $USER   # then log out and back in
```

### 2. Create the Cloudflare Tunnel

1. Cloudflare dashboard → **Zero Trust → Networks → Tunnels → Create a tunnel** → *Cloudflared* → name it `homeops`.
2. On the "Install connector" step choose **Docker** and copy the token (the long string after `--token`).
3. **Public hostname**: subdomain `home`, your domain, service **HTTP** → `app:8080`.

Recommended Cloudflare settings for this hostname:
- **Do not** enable Rocket Loader or Email Obfuscation (they rewrite pages).
- Leave caching at defaults. The app sends correct `Cache-Control` headers, so `sw.js` and `index.html` are never cached and hashed assets are cached forever.
- If you put **Cloudflare Access** in front, add a *Bypass* policy for `/sw.js`, `/manifest.webmanifest` and `/api/*`. Otherwise service-worker updates and push re-subscription can fail when the Access session expires. The app already has its own login with rate limiting, so Access is optional.

### 3. Configure and start

```sh
cp .env.example .env
nano .env        # set HOMEOPS_PUBLIC_URL, HOMEOPS_VAPID_EMAIL, POSTGRES_PASSWORD, S3_ACCESS_KEY, S3_SECRET_KEY, TUNNEL_TOKEN
make up          # or: docker compose up -d --build
```

Open `https://home.<your-domain>`. The first visit shows **Set up your household**, and whoever does it becomes the admin.
> ⚠️ Until setup is done, anyone who finds the URL could claim the household, so do it right after `make up`. Once one account exists, setup is closed for good.
Then go to **Settings → Household → Invite** and send the link to your partner.

### 4. On each phone

- **Android (Chrome):** open the site → menu → *Install app*. Then **Settings → Notifications → Enable**.
- **iPhone (iOS 16.4+):** open in Safari → Share → *Add to Home Screen*. Open HomeOps **from the Home Screen icon**, then **Settings → Notifications → Enable**. iOS only allows web push for installed apps.
- Add a passkey under **Settings → Sign-in & security** for Face ID / fingerprint sign-in.
- Use **Send test** on the notifications page to confirm delivery to every device.

## Day-to-day

| Task | Command |
| --- | --- |
| Update after pulling new code | `make update` |
| Logs | `make logs` |
| Backup now | `make backup-now` (nightly at 02:30, kept 30 days, in `./backups`: `homeops-*.sql.gz` = database, `files-*.tar.gz` = SeaweedFS data/photos) |
| Restore database | `./infra/restore.sh backups/homeops-YYYY-MM-DD.sql.gz` |
| Restore photos | `docker compose stop seaweedfs && docker run --rm -v homeops_seaweed:/data -v $PWD/backups:/b alpine sh -c "rm -rf /data/* && tar -xzf /b/files-YYYY-MM-DD.tar.gz -C /data" && docker compose start seaweedfs` |

Back up `./backups` somewhere off the server too (offsite target is still to be decided: B2, Google Drive via rclone, or a USB disk).

## How it works

- **Sprints:** one week, starting on the day you choose. With *auto-roll* on, the server completes the sprint when its week ends, carries unfinished tasks over and starts the next. There is always a "next" sprint to plan into.
- **Recurring chores:** daily / weekly (chosen weekdays) / monthly (day N, clamped to month end), every N periods, assigned to someone or rotating. Occurrences are generated into the current and next sprint. Deleting one does not bring it back.
- **Notifications:** assignment (instant), 08:00 digest of today's tasks, 19:00 nudge for what's still open, and planning reminders 30 minutes before and at the start time. Each can be switched off per person. Quiet hours hold pushes until they end; the inbox always has everything.
- **Offline:** the app shell is precached and data is kept in IndexedDB. Edits made offline are queued and replayed on reconnect, even if the app was closed in between.
- **Meal plan & suggestions:** each slot can hold several dishes (dal + sabzi + rice). Suggestions are rule-based: time since you last had a dish, how often you cook it, favourites, weekday habits ("biryani on Sundays"), what's in the pantry, variety across the week, and which dishes you usually eat together. Auto-fill builds full plates from these. It gets noticeably better after 2–4 months of history.
- **Grocery list:** built from planned, uncooked meals: scaled to servings, units converted and summed, pantry stock subtracted, rounded up to what you'd buy, grouped by aisle (move an item to another aisle once and it stays there). Finishing a trip stocks the pantry and records the bill for Phase 3's budget. Ticking items off works offline.
- **Pantry:** marking a meal cooked deducts its ingredients (undo restores them). Low-stock and expiry badges, a one-tap restock to the grocery list, and a 09:00 "use soon" notification.
- **Recipe import:** paste a link from almost any recipe site; the schema.org data sites publish for Google is parsed into ingredients, steps, times and photo, for you to review before saving. The server refuses to fetch addresses on your home network.
- **Budget:** each category has a monthly limit (override any single month; optional rollover of unspent money). Shared categories are the household budget; personal categories, transactions, accounts and bills are visible only to their owner, enforced in every query. Alerts at 80% and 100% go to both of you for shared categories, to the owner for personal ones.
- **Statement import:** CSV, XLSX, the HTML ".xls" many Indian banks serve, and HDFC's fixed-width text download. Columns are detected from headers (HDFC, ICICI, SBI, Axis and card layouts) with a manual column mapper as fallback, remembered per account. HDFC text statements are checked against the bank's own summary (debit/credit totals and counts). Rows are de-duplicated across overlapping imports, transfers (to yourselves, card bills, CRED, NPS/SIPs) are excluded from budgets, and fixing a category teaches a rule for next time. Every import can be undone.
- **Grocery invoices:** upload the PDF from the Zepto or Blinkit order page: the total (summed across Blinkit's merged seller invoices), line items, pack sizes and fees are read, the PDF is kept as the receipt, and goods can go straight into the pantry. When the bank statement arrives later, the matching UPI payment is recognised and skipped.
- **Bills:** due day, usual amount and reminder lead time; "Mark paid" records the expense and stops the month's reminders.
- **Retro:** one per sprint, from the Retro page or the "Complete sprint" dialog. While collecting, each of you sees only your own cards (the API returns just a count of the other's). Reveal together, vote, give action items an owner, and closing the retro creates them as tasks in the next sprint (voted ones as high priority).
- **Insights** (Sprints page): burndown of the current sprint against an ideal line, velocity (planned vs done) and fairness (who completed what) per sprint. Tasks without points count as 1.
- **Streaks & kudos:** a streak is consecutive days with at least one task done (today only breaks it once it's over). Send kudos from a task your partner finished, or from the Retro page; they get a notification.
- **Live sync:** one SSE stream per open app. A change on one phone shows up on the other in well under a second.

## Development

No Docker needed:

```sh
make dev-db    # terminal 1: embedded Postgres on :5433
make dev-api   # terminal 2: API on :8080 (photos go to backend/data without S3 settings)
make dev-web   # terminal 3: http://localhost:5173
make test
```

Requires Go 1.27+ and Node 24+.
