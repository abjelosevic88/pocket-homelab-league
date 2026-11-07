# Configuration

Environment variables; every one has a default. Personal settings (notifications, email, connections, AI) live in the UI per person — the `OPENAI_*`/`NOTIFY_*` values below are copied once into the first admin's settings.

| Variable | Default | What it does |
|---|---|---|
| `BASE_URL` | – | Where the app is (links in pushes, events and other apps, the OIDC callback) |
| `HOST` / `PORT` | `0.0.0.0` / `8080` | Where it listens |
| `DATA_DIR` | `./data` | The database and the Pocket Core token |
| `DB_PATH` | `$DATA_DIR/pocket-league.db` | The SQLite file |
| `AUTH_MODE` | `local` | `local` (passwords), `proxy` (a header from your reverse proxy), `none` (one user, for trying it out) |
| `SECURE_COOKIES` | `false` | Set `true` behind HTTPS |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` / `ADMIN_NAME` | – | The first account, made on an empty database |
| `OIDC_ISSUER` / `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` | – | Sign in with an OpenID Connect provider (not needed with Pocket Core) |
| `NOTIFY_URL` / `NOTIFY_FORMAT` | – / `apprise` | Pushes: an Apprise URL, an ntfy topic or a JSON webhook |
| `OPENAI_API_KEY` / `OPENAI_MODEL` / `OPENAI_BASE_URL` | – | AI without Pocket Core |
| `CORE_URL` / `CORE_REGISTRATION_KEY` / `CORE_APP_URL` | – | Pocket Core |
| `TRUST_PROXY` | `false` | Take the client address from `X-Forwarded-For` (rate limits per address) |
| `METRICS_ENABLED` | `false` | Prometheus `/metrics` |
| `MAX_BODY_BYTES` | `67108864` | Largest import |
| `DEMO_DATA` | `false` | Seed three groups and four months of games into the owner account (no email leaves) |
| `SESSION_TTL` | `720h` | How long a password sign-in lasts (a player's sign-in link: at least 180 days) |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `text` | |

Connections to Pocket Calendar, Health, Habits, Split, Money, Weather, Places, Contacts, Mail and Tasks are made per person under Settings → Connected apps: through Pocket Core (nothing to paste) or directly with the app's URL and an API token. How emails to players go out (Pocket Mail, your own SMTP server, or none) is under Settings → Email, per person.
