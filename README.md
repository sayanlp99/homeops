# HomeOps

A private household app for two: weekly sprints, chores, meal planning and budget.
Runs on your home server and is published through a Cloudflare Tunnel, so no ports are opened on your router.

**Phase 1 (done):** sign-in with password + passkeys, partner invite, installable PWA, push notifications, live sync, offline mode, Kanban board, sprint planning, recurring chores, notification inbox.
**Phase 2 (done):** weekly meal plan (breakfast/lunch/snacks/dinner, several dishes per slot), recipe library with photos and import-from-link, rule-based suggestions and auto-fill, grocery list built from the plan, pantry with low-stock and expiry tracking, meal & prep notifications.
**Coming:** Phase 3 budget & bank imports · Phase 4 retro & charts.

## Stack

| Layer | Choice |
| --- | --- |
| API | Go (chi, pgx), single ~15 MB binary in a distroless image |
| DB | PostgreSQL 17 |
| Files | SeaweedFS (S3 API) for recipe photos and, later, receipts — internal only, served through the app behind sign-in |
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
