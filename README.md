# ClickNest

Product analytics that instruments itself. Drop in one script tag — every click, pageview, and form submission is captured automatically, and AI turns the raw events into human-readable names by reading your source code.

Instead of `button.click #submit-form`, you see **"User clicked 'Place Order' on Checkout Page."**

No manual instrumentation. No tagging every button. No ops overhead — ships as a **single Go binary** with everything embedded, zero external dependencies.

## Features

- **Autocapture** — clicks, pageviews, form inputs, and submissions captured automatically by the SDK
- **Error tracking** — captures uncaught JS exceptions and promise rejections with stack traces
- **AI event naming** — LLM reads your DOM context (and optionally your source code via GitHub) to turn `button.click #checkout` into "User clicked 'Place Order'"
- **Funnels** — multi-step conversion analysis with cohort breakdowns
- **Path analysis** — page transition flows (where do users go next?)
- **Retention** — weekly cohort retention curves
- **Heatmaps** — click density visualization per page
- **Feature flags** — CRUD flags with rollout % and SDK `isEnabled()` check
- **Alerts** — metric threshold alerts with webhook delivery
- **Dashboards** — custom metric dashboards
- **CSV export** — one-click export from any data view
- **AI chat** — natural language queries against your analytics data
- **Backup & restore** — export/import your full database as a `.tar.gz`

---

## Deploy in 30 seconds

**Docker Compose (recommended):**
```bash
curl -O https://raw.githubusercontent.com/danielleslie/clicknest/main/docker-compose.yml
docker compose up -d
```
Open [http://localhost:8080](http://localhost:8080). Your API key is printed in the logs (`docker compose logs clicknest`).

**Pre-built binary:**
```bash
# Linux (amd64)
curl -L https://github.com/danielleslie/clicknest/releases/latest/download/clicknest_linux_amd64 -o clicknest
chmod +x clicknest && ./clicknest -data ./data

# Linux (arm64 — Raspberry Pi, Hetzner ARM, etc.)
curl -L https://github.com/danielleslie/clicknest/releases/latest/download/clicknest_linux_arm64 -o clicknest
chmod +x clicknest && ./clicknest -data ./data
```

**Fly.io (free tier):**
```bash
cp fly.toml.example fly.toml
fly launch   # detects Dockerfile automatically, skip all add-ons
fly volumes create clicknest_data --size 1 --region <your-region>
fly deploy --primary-region <your-region>
```

---

## First run

On first boot ClickNest automatically creates a default project and prints its API key to stdout:

```
ClickNest started on :8080 (dev=false, data=./data)
Created default project: default (API key: cn_abc123...)
```

**There is no username or password.** The dashboard at `http://localhost:8080` is open to anyone who can reach the server — protect it with a firewall, VPN, or authenticating reverse proxy before exposing it to the internet.

The API key is only needed for the SDK snippet you add to your site. You can always find it again in **Settings → Project** inside the dashboard.

---

## Add the SDK

Add the snippet to your site using the API key from the logs:

```html
<script src="http://localhost:8080/sdk.js"
  data-api-key="cn_YOUR_KEY"
  data-host="http://localhost:8080">
</script>
```

Or via npm:

```js
import ClickNest from '@clicknest/sdk';

ClickNest.init({
  apiKey: 'cn_YOUR_KEY',
  host: 'http://localhost:8080',
});
```

---

## Self-Hosting

No Postgres, no Redis, no Kafka. One binary, one port, two embedded databases.

> **Security note:** ClickNest is single-tenant with no built-in login. The dashboard is accessible to anyone who can reach the server. **Do not expose port 8080 directly to the internet.** Protect it with a firewall, VPN, or an authenticating reverse proxy (e.g. [Authelia](https://www.authelia.com/), Cloudflare Access, or basic auth via Nginx).

### Server requirements

| Traffic | RAM | CPU | Disk | ~Cost/mo |
|---|---|---|---|---|
| Personal / small site (<10k events/day) | 512 MB | 1 vCPU | 1 GB | ~$4 |
| Small–medium app (<100k events/day) | **1 GB** | 1 vCPU | 10 GB | ~$6 |
| Larger app with AI naming + GitHub sync | 2 GB | 2 vCPU | 20 GB | ~$12 |

The **1 GB / 1 vCPU** tier covers most self-hosted use cases.

<details>
<summary>Measured memory usage</summary>

| State | RSS |
|---|---|
| Idle (just started, no data) | ~30 MB |
| Light load (events + dashboard queries) | ~75 MB |
| Steady-state (small-medium app) | ~100–200 MB |
| Heavy (millions of stored events, complex queries) | ~300–500 MB |

DuckDB memory-maps data files, so RSS scales with query complexity, not total data volume.
</details>

### Deploy with Docker Compose

```bash
# Download compose file
curl -O https://raw.githubusercontent.com/danielleslie/clicknest/main/docker-compose.yml

# Start (runs in background, data persists in a Docker volume)
docker compose up -d

# View logs / find your API key
docker compose logs clicknest

# Stop
docker compose down
```

To pass optional env vars (GitHub OAuth):
```bash
GITHUB_CLIENT_ID=xxx GITHUB_CLIENT_SECRET=yyy docker compose up -d
```

### Deploy to a VPS

```bash
# Download binary
curl -L https://github.com/danielleslie/clicknest/releases/latest/download/clicknest_linux_amd64 \
  -o /usr/local/bin/clicknest
chmod +x /usr/local/bin/clicknest

# Create data directory
mkdir -p /var/lib/clicknest

# Run
clicknest -addr :8080 -data /var/lib/clicknest
```

**Run as a systemd service:**

```ini
# /etc/systemd/system/clicknest.service
[Unit]
Description=ClickNest Analytics
After=network.target

[Service]
ExecStart=/usr/local/bin/clicknest -addr :8080 -data /var/lib/clicknest
Restart=always
RestartSec=5
User=nobody

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now clicknest
```

### Deploy to Fly.io (free)

Fly's free tier includes 3 shared-cpu-256mb machines and 3GB of volume storage — enough to run ClickNest at no cost.

```bash
# Install flyctl: https://fly.io/docs/hands-on/install-flyctl/
fly auth login

# Copy the example config — fly launch will fill in your app name and region
cp fly.toml.example fly.toml
fly launch          # follow the wizard — skip all add-ons (no Postgres, Redis, etc.)
```

The wizard updates `fly.toml` with your app name and region. Then create the persistent volume **in the same region** and deploy:

```bash
# Replace 'ord' with your primary_region from fly.toml
fly volumes create clicknest_data --size 1 --region ord
fly deploy --primary-region ord
```

Find your API key in the logs:
```bash
fly logs
```

Set optional env vars:
```bash
fly secrets set GITHUB_CLIENT_ID=xxx GITHUB_CLIENT_SECRET=yyy
```

### HTTPS with Nginx

```nginx
server {
    listen 443 ssl;
    server_name analytics.yourdomain.com;

    ssl_certificate     /etc/letsencrypt/live/analytics.yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/analytics.yourdomain.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Required for live event SSE stream
        proxy_buffering off;
        proxy_cache off;
    }
}
```

Update your SDK snippet to use your domain:
```html
<script src="https://analytics.yourdomain.com/sdk.js"
  data-api-key="cn_YOUR_KEY"
  data-host="https://analytics.yourdomain.com">
</script>
```

### Disk usage

Events are stored in DuckDB, which compresses columnar data well:

| Events/day | Monthly growth |
|---|---|
| 10k | ~50 MB/mo |
| 100k | ~500 MB/mo |
| 1M | ~5 GB/mo |

---

## Configuration

| Flag | Default | Description |
|---|---|---|
| `-addr` | `:8080` | Listen address |
| `-data` | `./data` | Data directory (DuckDB + SQLite files) |
| `-dev` | `false` | Development mode (no embedded frontend) |

| Env var | Description |
|---|---|
| `GITHUB_CLIENT_ID` | GitHub OAuth app client ID (enables OAuth flow in Settings) |
| `GITHUB_CLIENT_SECRET` | GitHub OAuth app client secret |
| `CLICKNEST_ENCRYPTION_KEY` | AES-256 hex key for encrypting API keys at rest (auto-generated if unset) |

All other configuration (LLM provider, GitHub repo, project settings) is done through the dashboard at `/platform/settings`.

---

## Architecture

- **Backend**: Go (single binary, stdlib HTTP server)
- **Frontend**: SvelteKit (static build, embedded via `//go:embed`)
- **Event storage**: DuckDB (embedded columnar DB)
- **Metadata**: SQLite (projects, AI name cache, config)
- **SDK**: TypeScript, <2 KB gzipped
- **AI**: Pluggable LLM (OpenAI, Anthropic, Ollama)

### How AI naming works

When events are captured, ClickNest computes a fingerprint from the DOM context (element tag, id, classes, parent path, URL). A background worker pool checks if that fingerprint has been named before. If not, it builds a prompt from the DOM context (and optionally matched source code from GitHub) and calls your configured LLM to generate a human-readable name. The name is cached and backfilled onto all matching events.

This happens asynchronously — events are stored immediately, names appear as they're generated.

---

## Building from source

Requires Go 1.23+ and Node 20+.

```bash
# Install dependencies and build everything
make build

# Run in development mode (hot-reload frontend)
make dev-all
```

---

## License

[GNU Affero General Public License v3.0](LICENSE) (AGPL-3.0)

You can self-host and modify ClickNest freely. If you run it as a network service with modifications, you must publish those modifications under the same license.
