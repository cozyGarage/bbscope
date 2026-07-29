<p align="center">
  <img src="logo.svg" alt="bbscope-logo-white" width="500">
</p>

> [!IMPORTANT]  
> **This is bbscope v2!**  
> 
> Major changes include a new command structure and powerful new features.
> 
> **Breaking Change:**
> Subcommands have changed!
> *   Old: `bbscope h1`
> *   **New:** `bbscope poll h1` (and similarly for `bc`, `it`, `ywh`, etc.)
> 
> **New Features in v2:**
> *   **PostgreSQL Support (Optional)**: Store targets in a DB for persistence and querying.
> *   **Changes Monitoring**: Track new and removed targets over time (requires DB).
> *   **AI Scope Normalization**: Use LLMs to clean up messy scope strings.
> *   **Docker Image**: Ready-to-use image on GHCR.
> *   **Centralized Polling**: Poll multiple platforms in one go.
> *   **Smart Scope Extraction**: Use `db get` to fetch normalized targets by category (wildcards, cidrs, etc.).

**bbscope** is a powerful scope aggregation tool for bug bounty hunters, designed to fetch, store, and manage program scopes from HackerOne, Bugcrowd, Intigriti, YesWeHack, and Immunefi right from your command line.

Visit [bbscope.com](https://bbscope.com/) to explore an hourly-updated list of public scopes from all supported platforms, stats, and more!

---

## ✨ Features

- **Multi-Platform Support**: Aggregates scopes from all major bug bounty platforms.
- **PostgreSQL Database**: Stores all scope data in a PostgreSQL database for reliable, concurrent access.
- **Powerful Querying**: Easily print targets by type (URLs, wildcards, mobile, etc.), platform, or program.
- **Track Changes**: Monitor scope additions and removals over time.
- **LLM Cleanup (opt-in)**: Let GPT-style models fix messy scope strings in bulk when polling.
- **Flexible Output**: Get your data in plain text, JSON, or CSV.

---

## 📦 Installation

Ensure you have a recent version of Go installed, then run:

```bash
go install github.com/cozyGarage/bbscope/v2@latest
```

### Docker Installation

You can also run `bbscope` using Docker. The Docker image is automatically built and published to GitHub Container Registry (GHCR) on every push.

**Pull the latest image:**
```bash
docker pull ghcr.io/cozygarage/bbscope:latest
```

**Run bbscope with Docker:**
```bash
docker run --rm ghcr.io/cozygarage/bbscope:latest [command] [flags]
```

**Tip:** Add `--pull=always` to the command if you want to automatically always use the latest bbscope version.

**Important:** To persist your configuration across container runs, bind-mount the config file:

```bash
# Run with config mounted
docker run --rm \
  -v ~/.bbscope.yaml:/root/.bbscope.yaml \
  ghcr.io/cozygarage/bbscope:latest poll --db -b -p
```

**Note:** The container connects to your PostgreSQL database using the `db_url` configured in `~/.bbscope.yaml`. Make sure your database is accessible from the container (use `host.docker.internal` for local databases on macOS/Windows, or your database's network address).

---

## 🔐 Configuration

`bbscope` stores credentials securely in your operating system's native keychain:
- **macOS**: Keychain
- **Windows**: Credential Manager
- **Linux**: Secret Service (GNOME Keyring, KWallet)

### Setting Up Credentials

Use the `config` command to manage credentials securely:

```bash
# Store credentials in OS keychain
bbscope config set hackerone.username
bbscope config set hackerone.token
bbscope config set bugcrowd.email
bbscope config set bugcrowd.password

# List stored credential keys
bbscope config list

# Get a stored credential
bbscope config get hackerone.username

# Delete a credential
bbscope config delete hackerone.token

# Migrate existing config file credentials to keychain
bbscope config migrate
```

### Config File (Legacy/Fallback)

A configuration file at `~/.bbscope.yaml` is still supported for database URL and as a fallback. The config command will prompt for values securely without echoing to the terminal.

```yaml
# PostgreSQL connection URL
db_url: "postgres://user:password@localhost:5432/bbscope?sslmode=disable"

# AI configuration (optional)
ai:
  provider: "openai"
  api_key: "" # or set OPENAI_API_KEY env var
  model: "gpt-4o-mini"
  max_batch: 25
  max_concurrency: 10
```

Alternatively, you can provide credentials directly via command-line flags when running a `poll` subcommand. Flags will always override values in the configuration file and keychain.

**Authentication Flags for `poll` Subcommands:**

| Command | Flag | Description |
| --- | --- | --- |
| `poll h1` | `--user`, `--token` | Your HackerOne username and API token. |
| `poll bc` | `--token` | A live `_bugcrowd_session` cookie. Use as an alternative to email/pass/otp. |
| | `--email`, `--password`, `--otp-secret` | Your Bugcrowd login credentials. |
| `poll it` | `--token` | Your Intigriti authorization token (Bearer). |
| `poll ywh` | `--token` | A live YesWeHack bearer token. Use as an alternative to email/pass/otp. |
| | `--email`, `--password`, `--otp-secret`| Your YesWeHack login credentials. |

**Database Configuration (Optional):**

To use bbscope's database features (like tracking changes or querying targets), you need a PostgreSQL database.

**Option 1: Use an existing PostgreSQL instance**

Add your connection string to `~/.bbscope.yaml`:

```yaml
db_url: "postgres://user:password@localhost:5432/bbscope?sslmode=disable"
```

**Option 2: Quick setup with Docker**

> [!IMPORTANT]
> Replace `<YOUR_SECURE_PASSWORD>` with a strong, unique password.

```bash
docker run --name bbscope-db \
  -e POSTGRES_USER=bbscope \
  -e POSTGRES_PASSWORD=<YOUR_SECURE_PASSWORD> \
  -e POSTGRES_DB=bbscope \
  -p 5432:5432 \
  -d postgres:alpine
```

Then add to your `~/.bbscope.yaml`:

```yaml
db_url: "postgres://bbscope:<YOUR_SECURE_PASSWORD>@localhost:5432/bbscope?sslmode=disable"
```

Tables are automatically created on the first run.

### Notifications (Optional)

Configure notifications to receive alerts when scope changes are detected:

```yaml
notifications:
  slack:
    webhook: "https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
    events: ["added", "removed"]
  
  discord:
    webhook: "https://discord.com/api/webhooks/YOUR/WEBHOOK/URL"
    events: ["added"]
  
  telegram:
    bot_token: "YOUR_BOT_TOKEN"
    chat_id: "YOUR_CHAT_ID"
  
  email:
    smtp_host: "smtp.gmail.com"
    smtp_port: 587
    from: "alerts@example.com"
    to: ["security@example.com"]
    username: "alerts@example.com"
    password: "app-password"
  
  webhook:
    url: "https://api.example.com/webhooks/bbscope"
    headers:
      Authorization: "Bearer TOKEN"
```

See **Notification Integrations** section below for setup guides.

---

## 🛠️ Usage & Commands

`bbscope` is organized around a few top-level commands: `poll` (fetch scopes), `db` (query/manage stored data), `daemon` (scheduled polling), and `tui` (interactive terminal UI).

### `poll` - Fetching Scopes

The `poll` command fetches scope data from the platforms. You can poll all platforms at once or specify which ones to poll.

**Subcommands:**

- `bbscope poll`: Polls all configured platforms.
- `bbscope poll h1`: Polls HackerOne.
- `bbscope poll bc`: Polls Bugcrowd.
- `bbscope poll it`: Polls Intigriti.
- `bbscope poll ywh`: Polls YesWeHack.
- `bbscope poll immunefi`: Polls Immunefi (no authentication required).



**Flags for `poll`:**

| Flag | Description | Default |
| --- | --- | --- |
| `--db` | Save results to the database and print changes. | `false` |
| `--ai` | Normalize messy targets in bulk using an LLM before writing to the DB (requires API key). | `false` |
| `-b, --bbp-only` | Only fetch programs offering monetary rewards. | `false` |
| `-p, --private-only` | Only fetch data from private programs. | `false` |
| `--category` | Scope categories to include (e.g., `url`, `cidr`, `mobile`). | `"all"` |
| `-o, --output` | Output flags for printing to stdout (`t`=target, `d`=description, `c`=category, `u`=program URL). | `"tu"` |
| `-d, --delimiter` | Delimiter for `txt` output when using multiple output flags. | `" "` |
| `--oos` | Include out-of-scope targets in the output. | `false` |
| `--concurrency`| Number of concurrent fetches per platform. | `5` |

---

### `daemon` - Scheduled Polling

The `daemon` command runs bbscope as a background service that polls platforms on a schedule.

**Usage:**
```bash
bbscope daemon --interval DURATION [flags]
```

**Flags for `daemon`:**

| Flag | Description | Default |
| --- | --- | --- |
| `--interval` | Polling interval (e.g., 30m, 1h, 2h) | `1h` |
| `--platforms` | Comma-separated list of platforms (e.g., h1,bc,it) | `all` |
| `--db` | Save results to database | `false` |
| `--ai` | Use AI normalization | `false` |
| `--pid-file` | Write process ID to file | `""` |

**Examples:**
```bash
# Poll all platforms every hour
bbscope daemon --interval 1h --db

# Poll specific platforms every 30 minutes with AI
bbscope daemon --interval 30m --platforms h1,bc --db --ai

# Production mode with PID file
bbscope daemon --interval 1h --platforms h1,bc,it,ywh --db --pid-file /var/run/bbscope.pid
```

**Systemd Integration:**
```ini
[Unit]
Description=bbscope Daemon
After=network.target postgresql.service

[Service]
Type=simple
ExecStart=/usr/local/bin/bbscope daemon --interval 1h --db --ai
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

#### AI-Assisted Normalization (Experimental)

Messy scope entries from platforms can now be cleaned up in bulk with the `--ai` flag:

```bash
bbscope poll --db --ai
```

- Requires an API key in `~/.bbscope.yaml` under the `ai` section (or the `OPENAI_API_KEY` environment variable).
- Entries are batched per program to minimize API calls.
- If the model fails or omits an entry, the original target is used so nothing is lost.
- When the text explicitly says "out of scope" or similar, the AI pass can flip the entry's `in_scope` flag for you.
- Tune throughput with `max_batch` (targets per request) and `max_concurrency` (simultaneous requests) in the `ai` config section.

---

### `db` - Interacting with the Database

The `db` command lets you query and manage the data stored in your PostgreSQL database.

#### `db print`

Prints raw scope data from the database. This is best for inspecting what's currently stored or for exporting data to other formats (JSON/CSV).

**Usage:** `bbscope db print [flags]`

#### `db get`

Extracts specific types of targets from the database. This command is designed for piping into other tools (like `subfinder`, `httpx`, etc.).

**Usage:** `bbscope db get [subcommand] [flags]`

**Subcommands:**

- `wildcards`: Get all wildcard domains (e.g., `*.example.com`).
- `domains`: Get all domains (including wildcards). IPs and CIDR ranges are excluded.
- `urls`: Get all full URLs (http/https).
- `cidrs`: Get all CIDR ranges (and IP ranges).
- `ips`: Get all IP addresses.

All `db get` subcommands accept `--platform` to limit results to a single platform (e.g. `--platform h1`). It defaults to `all`.

**Flags for `db get wildcards`:**

| Flag | Description | Default |
| --- | --- | --- |
| `--platform` | Filter by platform. | `"all"` |
| `-a, --aggressive` | aggressive mode: extract root domains from URLs and include them too. | `false` |
| `-o, --output` | Output flags (`t`=target, `u`=program URL). | `"t"` |
| `-d, --delimiter` | Delimiter for output. | `" "` |

**Example:**
```bash
# Get all wildcards and pipe to another tool
bbscope db get wildcards --aggressive | subfinder
```

#### `db stats`

Shows high-level statistics about the data in the database.

**Usage:** `bbscope db stats`

#### `db changes`

Shows the most recent scope changes (additions/removals).

**Usage:** `bbscope db changes`

| `--limit` | Number of recent changes to show. | `50` |

#### `db diff`

Compare scope between two dates to track changes over time.

**Usage:** `bbscope db diff --from 2024-01-01 [flags]`

| Flag | Description |
| --- | --- |
| `--from` | Start date (YYYY-MM-DD) (required) |
| `--to` | End date (YYYY-MM-DD) (defaults to now) |
| `--program` | Filter by program URL (partial match) |
| `--only-added` | Show only additions |
| `--only-removed` | Show only removals |
| `--format` | Output format: `text`, `json`, `csv` |

#### `db export`

Export all database content to JSON or CSV.

**Usage:** `bbscope db export [flags] > file`

| Flag | Description |
| --- | --- |
| `--format` | Output format: `json` (default), `csv` |
| `--platform` | Filter by platform |

#### `db import`

Import data from a file (backup restoration or custom targets).

**Usage:** `bbscope db import --file backup.json`

| Flag | Description |
| --- | --- |
| `--file` | Input file (defaults to stdin) |
| `--format` | Input format: `json` (default), `csv` |

#### `db find`

Search for a string in current and historical scopes.

**Usage:** `bbscope db find [query]`

#### `db shell`

Open a `psql` shell connected to your database.

**Usage:** `bbscope db shell`

The bbscope DB schema will also be printed to stdout for ease of reference.

#### `db add`

Add a custom target to the database manually.

**Usage:** `bbscope db add --target <target> [flags]`

**Flags for `db add`:**

| Flag | Description | Default |
| --- | --- | --- |
| `-t, --target` | Target to add (can be comma-separated). | |
| `-c, --category` | Category of the target (e.g., `wildcard`, `url`). | `"wildcard"` |
| `-u, --program-url` | Program URL to associate with the target. | `"custom"` |

---

### `tui` - Interactive Terminal UI

Launch an interactive terminal dashboard to browse stored scope, view stats, and inspect recent changes without memorizing `db` flags. Requires the database to be configured.

**Usage:** `bbscope tui`

See [docs/TUI_QUICKREF.md](docs/TUI_QUICKREF.md) for keyboard shortcuts.

---

## 📖 Examples

**1. Run Daemon - Automated Polling**

Poll all platforms automatically every hour:

```bash
bbscope daemon --interval 1h --db -b -p
```

**2. Daemon with Specific Platforms**

Poll only HackerOne and Bugcrowd every 30 minutes:

```bash
bbscope daemon --interval 30m --platforms h1,bc --db --ai
```

**3. First-Time Setup: Poll all private, bounty-only programs and save to DB**

This is a great first command to run to populate your database.

```bash
bbscope poll --db -b -p
```

**4. Get all wildcards from all platforms, aggressive extraction**

```bash
bbscope db get wildcards -a
```

**5. Get all URLs and pipe to httpx**

```bash
bbscope db get urls | httpx 
```

**6. Show the 10 most recent scope changes**

```bash
bbscope db changes --limit 10
```

**7. Get all CIDRs from all platforms**

```bash
bbscope db get cidrs
```

**8. Poll with a Proxy**

```bash
# Great for debugging when a platform changes their API 
bbscope poll h1 --proxy "http://127.0.0.1:8080"
```

**9. HackerOne Polling (with Auth)**

```bash
# Using flags (overrides config file)
bbscope poll h1 --user "your_user" --token "your_token"
```

**10. Bugcrowd Polling**

```bash
# Using session token
bbscope poll bc --token "your_bugcrowd_session"

# Using credentials
bbscope poll bc --email "..." --password "..." --otp-secret "..."
```

**11. Intigriti Polling**

```bash
bbscope poll it --token "your_api_token"
```

**12. YesWeHack Polling**

```bash
bbscope poll ywh --token "your_jwt_token"
```

**13. Immunefi Polling**

```bash
bbscope poll immunefi
```
