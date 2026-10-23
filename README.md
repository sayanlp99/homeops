# HomeOps

A private, self-hosted household app: chores in weekly sprints, meal planning with
recipes, grocery and pantry, and a shared budget with private personal spending.
Several households, each with owner/admin/member/viewer roles. It runs on a home
server behind a Cloudflare Tunnel, with no open router ports, and installs on phones
as an app with push notifications and offline use.

## Features

- **Tasks:** Kanban board, weekly sprints with auto-roll, backlog planning, recurring
  chores (fixed or rotating), labels, due-date digests.
- **Kitchen:**
  - Meal plan for breakfast, lunch, snacks and dinner, with several dishes per slot.
  - Rule-based suggestions and auto-fill.
  - Recipe library with import from links (incl. BongEats).
  - Grocery list built from the plan; pantry with low-stock and expiry tracking.
- **Budget:**
  - Monthly envelopes, shared and private.
  - Quick expense entry with receipt photos.
  - Bank statement import (HDFC, ICICI, SBI, Axis, cards) with learned
    categorisation.
  - Zepto and Blinkit invoice import, bills with reminders, budget alerts.
- **Rituals:** sprint retros with private cards, votes and action items; burndown,
  velocity and fairness charts; streaks and kudos.
- **Everyone:**
  - Sign in with a password or passkey; email invites and password reset.
  - Several households per person.
  - Web Push on Android, iOS (installed) and desktop.
  - Live sync between devices, and works offline.

## Quick start

```sh
./run-local.sh        # http://localhost:8080 — embedded Postgres, no Docker (Go 1.27+, Node 24+)
```

To run it for real on a home server, see [docs/deployment.md](docs/deployment.md).

## Documentation

| | |
| --- | --- |
| [User guide](docs/user-guide.md) | Using HomeOps: households, roles, phones, every feature |
| [Deployment](docs/deployment.md) | Installing and updating on a home server, configuration |
| [Operations](docs/operations.md) | Logs, backups, restoring, troubleshooting |
| [Security](docs/security.md) | Firewall, SSH, intrusion and miner watchdog, secrets |
| [Mail](docs/mail.md) | Self-hosted mail for your domain |
| [Architecture](docs/architecture.md) · [Development](docs/development.md) · [API](docs/api.md) | For developers |

## Stack

Go (chi, pgx) · PostgreSQL · SeaweedFS (S3) · React 19 + Vite + TanStack Query +
Tailwind · Workbox service worker · Server-Sent Events · Web Push (VAPID) ·
systemd + Cloudflare Tunnel.
