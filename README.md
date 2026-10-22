# HomeOps

A private household app for two: weekly sprints, chores, meal planning and budget.
Runs on your home server and is published through a Cloudflare Tunnel, so no ports are opened on your router.

**Phase 1 (done):** sign-in with password + passkeys, partner invite, installable PWA, push notifications, live sync, offline mode, Kanban board, sprint planning, recurring chores, notification inbox.
**Coming:** Phase 2 meals, recipes, pantry & grocery · Phase 3 budget & bank imports · Phase 4 retro, charts, suggestions.

## Stack

| Layer | Choice |
| --- | --- |
| API | Go (chi, pgx), single ~15 MB binary in a distroless image |
| DB | PostgreSQL 17 |
| Web | React 19 + Vite + TanStack Query + Tailwind; Workbox service worker |
| Realtime | Server-Sent Events (works through Cloudflare Tunnel) |
| Push | Standard Web Push with VAPID keys generated on first start |

## Deploy on the home server

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
nano .env        # set HOMEOPS_PUBLIC_URL, HOMEOPS_VAPID_EMAIL, POSTGRES_PASSWORD, TUNNEL_TOKEN
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
| Backup now | `make backup-now` (nightly backups run automatically at 02:30, kept 30 days, in `./backups`) |
| Restore | `./infra/restore.sh backups/homeops-YYYY-MM-DD.sql.gz` |

Back up `./backups` somewhere off the server too (offsite target is still to be decided: B2, Google Drive via rclone, or a USB disk).

## How it works

- **Sprints:** one week, starting on the day you choose. With *auto-roll* on, the server completes the sprint when its week ends, carries unfinished tasks over and starts the next. There is always a "next" sprint to plan into.
- **Recurring chores:** daily / weekly (chosen weekdays) / monthly (day N, clamped to month end), every N periods, assigned to someone or rotating. Occurrences are generated into the current and next sprint. Deleting one does not bring it back.
- **Notifications:** assignment (instant), 08:00 digest of today's tasks, 19:00 nudge for what's still open, and planning reminders 30 minutes before and at the start time. Each can be switched off per person. Quiet hours hold pushes until they end; the inbox always has everything.
- **Offline:** the app shell is precached and data is kept in IndexedDB. Edits made offline are queued and replayed on reconnect, even if the app was closed in between.
- **Live sync:** one SSE stream per open app. A change on one phone shows up on the other in well under a second.

## Development

No Docker needed:

```sh
make dev-db    # terminal 1: embedded Postgres on :5433
make dev-api   # terminal 2: API on :8080
make dev-web   # terminal 3: http://localhost:5173
make test
```

Requires Go 1.27+ and Node 24+.
