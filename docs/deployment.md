# InstantF Bot-X — Production Deployment Guide

### Option 2: Docker Compose on a DigitalOcean Droplet

> **Stack:** 10 Go microservices · CockroachDB · Valkey (Redis) · Docker Compose · Nginx + SSL  
> **Target:** DigitalOcean Droplet (4GB RAM / 2 vCPUs minimum)

---

## Table of Contents

1. [Overview & Architecture](#1-overview--architecture)
2. [Prerequisites](#2-prerequisites)
3. [Phase 1 — Provision Backing Infrastructure](#3-phase-1--provision-backing-infrastructure)
   - [1.1 CockroachDB](#11-database-cockroachdb)
   - [1.2 Valkey (Redis)](#12-cachepubsub-valkey-redis)
4. [Phase 2 — Provision the Droplet](#4-phase-2--provision-the-droplet)
   - [2.1 Create the Droplet](#21-create-the-droplet)
   - [2.2 SSH Into the Droplet](#22-ssh-into-the-droplet)
5. [Phase 3 — Configure the Project](#5-phase-3--configure-the-project)
   - [3.1 Clone the Repository](#31-clone-the-repository)
   - [3.2 Create the `.env` File](#32-create-the-env-file)
   - [3.3 Update `docker-compose.yml`](#33-update-docker-composeyml)
6. [Phase 4 — Build & Launch Services](#6-phase-4--build--launch-services)
7. [Phase 5 — Expose XGateway via Nginx + SSL](#7-phase-5--expose-xgateway-via-nginx--ssl)
8. [Phase 6 — Run Database Migrations](#8-phase-6--run-database-migrations)
9. [Post-Deployment Operations](#9-post-deployment-operations)
10. [Troubleshooting](#10-troubleshooting)

---

## 1. Overview & Architecture

```
Internet
   │
   ▼ HTTPS :443
┌─────────────────────────────────┐
│         Nginx (reverse proxy)   │  ← Droplet host
│         SSL via Certbot         │
└────────────────┬────────────────┘
                 │ proxy_pass :8081
                 ▼
┌─────────────────────────────────────────────────────┐
│              Docker Internal Network                 │
│                                                     │
│  ┌──────────┐  gRPC   ┌───────────┐  ┌──────────┐  │
│  │ xgateway │ ──────▶ │  giveaway │  │  payment │  │
│  │  :8081   │         │  :50051   │  │  router  │  │
│  └──────────┘         └───────────┘  └──────────┘  │
│                                                     │
│  ┌─────┐ ┌────────────┐ ┌───────┐ ┌─────────────┐  │
│  │ kyc │ │ compliance │ │ audit │ │ notification │  │
│  └─────┘ └────────────┘ └───────┘ └─────────────┘  │
│                                                     │
│  ┌────────────────┐  ┌───────┐  ┌──────────────┐   │
│  │ reconciliation │  │ entry │  │   xgateway   │   │
│  └────────────────┘  └───────┘  └──────────────┘   │
└─────────────────────────────────────────────────────┘
          │                        │
          ▼                        ▼
   CockroachDB               Valkey (Redis)
   (Managed Cloud)           (DO Managed DB)
```

**Key design decisions:**

- Only `xgateway` is exposed to the host network (port `8081`). All other services communicate internally over Docker's bridge network via gRPC.
- CockroachDB and Valkey are **external managed services** — they are not defined inside `docker-compose.yml`.
- All secrets are passed via a `.env` file at the project root.

---

## 2. Prerequisites

Before you begin, ensure you have the following ready on your **local machine**:

| Tool                      | Purpose                 | Install                                                                 |
| ------------------------- | ----------------------- | ----------------------------------------------------------------------- |
| `doctl` or browser access | DigitalOcean management | [docs.digitalocean.com](https://docs.digitalocean.com/reference/doctl/) |
| `ssh` + `ssh-keygen`      | Droplet access          | Pre-installed on macOS/Linux                                            |
| `golang-migrate`          | Running DB migrations   | See [Phase 6](#8-phase-6--run-database-migrations)                      |
| Your repo access          | Clone on Droplet        | GitHub SSH key or token                                                 |
| Domain name               | For SSL cert            | Any DNS provider                                                        |

---

## 3. Phase 1 — Provision Backing Infrastructure

> ⚠️ **Do this before creating your Droplet.** You need the connection strings from these services to populate your `.env` file later.

---

### 1.1 Database: CockroachDB

CockroachDB Serverless/Dedicated provides a distributed, highly available PostgreSQL-compatible database without you managing replication or backups.

**Steps:**

1. Go to [cockroachlabs.cloud](https://cockroachlabs.cloud) and sign in or create an account.

2. Click **Create Cluster** and choose your tier:
   - **Serverless** — Free tier, good for lower traffic
   - **Dedicated** — Recommended for consistent production workloads

3. Select a **cloud region** that matches where your Droplet will be. For example:
   - DigitalOcean Frankfurt → choose `gcp-europe-west3`
   - DigitalOcean NYC → choose `gcp-us-east1`

4. Once the cluster is live, navigate to **SQL Users** → **Add User**:
   - Username: `botx_user`
   - Save the auto-generated password securely (it won't be shown again)

5. Navigate to **Databases** → **Add Database**:
   - Database name: `instantf_bot_x`

6. Navigate to **Connect** → copy your connection string:

   ```
   postgresql://botx_user:YOUR_PASSWORD@free-tier.gcp-us-central1.cockroachlabs.cloud:26257/instantf_bot_x?sslmode=verify-full
   ```

   > 📋 **Save this as `DATABASE_URL`** — you'll paste it into `.env` in Phase 3.

---

### 1.2 Cache/PubSub: Valkey (Redis)

DigitalOcean's managed Valkey is a fully Redis-compatible key-value store. Your Go services use standard Redis clients (`go-redis`, etc.) that connect to it without any code changes.

**Steps:**

1. In the DigitalOcean dashboard, go to **Databases** → **Create Database Cluster**.

2. Select **Valkey** as the engine (it may still appear as "Redis" — they are functionally identical here).

3. Choose the **same region** as your planned Droplet.

4. Select a node plan:
   - **1GB RAM / 1 vCPU** — sufficient to start, scale later

5. Click **Create Database Cluster** and wait ~2–3 minutes for provisioning.

6. Once ready, click the cluster → go to **Connection Details** → copy the connection string:

   ```
   rediss://default:YOUR_PASSWORD@your-cluster.db.ondigitalocean.com:25061
   ```

   > 📋 **Save this as `REDIS_URL`** — you'll paste it into `.env` in Phase 3.

---

## 4. Phase 2 — Provision the Droplet

---

### 2.1 Create the Droplet

1. In DigitalOcean, click **Create** → **Droplets**.

2. Under **Choose an image**, click the **Marketplace** tab. Search for **Docker** and select:

   > **Docker** on Ubuntu 22.04 LTS

   This gives you a Droplet with Docker Engine and Docker Compose pre-installed.

3. **Choose a plan:** Select at minimum:

   | Spec         | Minimum   | Recommended |
   | ------------ | --------- | ----------- |
   | RAM          | 4 GB      | 8 GB        |
   | vCPUs        | 2         | 4           |
   | Disk         | 80 GB SSD | 160 GB SSD  |
   | Monthly cost | ~$24/mo   | ~$48/mo     |

4. **Choose a datacenter region:** Match this to your CockroachDB and Valkey regions.

5. **Authentication:** Add your SSH public key.

   If you don't have one, generate it locally:

   ```bash
   ssh-keygen -t ed25519 -C "your_email@example.com"
   # Press Enter for all prompts to accept defaults
   cat ~/.ssh/id_ed25519.pub
   # Copy the output and paste it into DigitalOcean
   ```

6. Set a hostname (e.g., `instantf-botx-prod`) and click **Create Droplet**.

7. Once created, **copy the public IP address** from the Droplet dashboard.

---

### 2.2 SSH Into the Droplet

From your local terminal:

```bash
ssh root@YOUR_DROPLET_IP
```

Verify Docker is ready:

```bash
docker --version
# Expected: Docker version 25.x.x or higher

docker compose version
# Expected: Docker Compose version v2.x.x or higher
```

---

## 5. Phase 3 — Configure the Project

---

### 3.1 Clone the Repository

Navigate to the `/opt` directory (conventional for app deployments) and clone:

```bash
cd /opt
git clone https://github.com/your-org/bot-x.git
cd bot-x
```

**If your repository is private**, set up a GitHub Deploy Key:

```bash
# On the Droplet, generate a deploy key
ssh-keygen -t ed25519 -C "instantf-botx-deploy" -f ~/.ssh/deploy_key -N ""

# Print the public key
cat ~/.ssh/deploy_key.pub
```

Then in GitHub: go to your repo → **Settings** → **Deploy Keys** → **Add deploy key** → paste the public key → check "Allow read access" → Save.

Configure SSH to use this key:

```bash
cat >> ~/.ssh/config << 'EOF'
Host github.com
  IdentityFile ~/.ssh/deploy_key
  IdentitiesOnly yes
EOF
```

Now clone using SSH:

```bash
git clone git@github.com:your-org/bot-x.git
```

---

### 3.2 Create the `.env` File

This is the most important configuration step. All 10 services load their configuration from this single file.

```bash
nano /opt/bot-x/.env
```

Paste the following template and fill in every value:

```env
# ============================================================
# APPLICATION
# ============================================================
APP_ENV=production

# ============================================================
# DATABASE — CockroachDB
# Connection string from Phase 1.1
# ============================================================
DATABASE_URL=postgresql://botx_user:YOUR_PASSWORD@your-cluster.cockroachlabs.cloud:26257/instantf_bot_x?sslmode=verify-full

# ============================================================
# CACHE / PUBSUB — Valkey (Redis)
# Connection string from Phase 1.2
# ============================================================
REDIS_URL=rediss://default:YOUR_PASSWORD@your-cluster.db.ondigitalocean.com:25061

# ============================================================
# SAFEHAVEN — RSA Private Key (multiline PEM)
# Paste the full key as a literal multiline string.
# Do NOT replace newlines with \n.
# ============================================================
SAFEHAVEN_PRIVATE_KEY_PEM="-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA...
...your key content...
-----END RSA PRIVATE KEY-----"

# ============================================================
# STRIPE
# ============================================================
STRIPE_SECRET_KEY=sk_live_YOUR_STRIPE_KEY

# ============================================================
# INTERNAL gRPC ADDRESSES
# These use Docker service names as hostnames.
# Do not change unless you rename services in docker-compose.yml
# ============================================================
GRPC_GIVEAWAY_ADDR=giveaway:50051
GRPC_PAYMENT_ROUTER_ADDR=payment-router:50051
GRPC_KYC_ADDR=kyc:50051
GRPC_COMPLIANCE_ADDR=compliance:50051
GRPC_AUDIT_ADDR=audit:50051
GRPC_NOTIFICATION_ADDR=notification:50051
GRPC_RECONCILIATION_ADDR=reconciliation:50051
GRPC_ENTRY_ADDR=entry:50051
GRPC_XGATEWAY_ADDR=xgateway:50051
```

Save and exit: `Ctrl+O` → `Enter` → `Ctrl+X`

Secure the file so only root can read it:

```bash
chmod 600 /opt/bot-x/.env
```

> ⚠️ **RSA Key note:** The PEM key must contain literal newlines (as shown above). Do **not** use `\n` escape sequences — Go's PEM parser requires real line breaks.

---

### 3.3 Update `docker-compose.yml`

Open your existing `docker-compose.yml`:

```bash
nano /opt/bot-x/docker-compose.yml
```

**Remove** any locally-defined services for `postgres`, `redis`, `vault`, or any other infrastructure you are now sourcing externally.

Your final `docker-compose.yml` should follow this structure:

```yaml
version: "3.9"

services:
  # ──────────────────────────────────────────
  # Public-facing gateway (only service with
  # a host port binding)
  # ──────────────────────────────────────────
  xgateway:
    build:
      context: .
      dockerfile: services/xgateway/Dockerfile
    ports:
      - "8081:8081" # Nginx proxies to this
    env_file:
      - .env
    restart: unless-stopped

  # ──────────────────────────────────────────
  # Internal services (gRPC only, no host ports)
  # ──────────────────────────────────────────
  giveaway:
    build:
      context: .
      dockerfile: services/giveaway/Dockerfile
    env_file:
      - .env
    restart: unless-stopped

  payment-router:
    build:
      context: .
      dockerfile: services/payment-router/Dockerfile
    env_file:
      - .env
    restart: unless-stopped

  kyc:
    build:
      context: .
      dockerfile: services/kyc/Dockerfile
    env_file:
      - .env
    restart: unless-stopped

  compliance:
    build:
      context: .
      dockerfile: services/compliance/Dockerfile
    env_file:
      - .env
    restart: unless-stopped

  audit:
    build:
      context: .
      dockerfile: services/audit/Dockerfile
    env_file:
      - .env
    restart: unless-stopped

  notification:
    build:
      context: .
      dockerfile: services/notification/Dockerfile
    env_file:
      - .env
    restart: unless-stopped

  reconciliation:
    build:
      context: .
      dockerfile: services/reconciliation/Dockerfile
    env_file:
      - .env
    restart: unless-stopped

  entry:
    build:
      context: .
      dockerfile: services/entry/Dockerfile
    env_file:
      - .env
    restart: unless-stopped
```

**Rules to follow:**

- Only `xgateway` has a `ports:` section. All other services must not bind host ports.
- Every service must have `env_file: - .env` and `restart: unless-stopped`.
- Docker Compose automatically creates a shared network — services reach each other by their service name (e.g., `giveaway:50051`).

---

## 6. Phase 4 — Build & Launch Services

### Build images and start all containers

```bash
cd /opt/bot-x
docker compose up -d --build
```

- `--build` — forces a fresh build of all Docker images
- `-d` — runs everything in detached (background) mode

This will take **5–15 minutes** on first run as it downloads base images and compiles all Go services.

### Verify all containers are running

```bash
docker compose ps
```

Expected output — all services should show `Up`:

```
NAME                    STATUS          PORTS
instantf-xgateway       Up 2 minutes    0.0.0.0:8081->8081/tcp
instantf-giveaway       Up 2 minutes
instantf-payment-router Up 2 minutes
instantf-kyc            Up 2 minutes
instantf-compliance     Up 2 minutes
instantf-audit          Up 2 minutes
instantf-notification   Up 2 minutes
instantf-reconciliation Up 2 minutes
instantf-entry          Up 2 minutes
```

### Check logs for errors

```bash
# Stream all logs
docker compose logs -f

# Stream logs for a single service
docker compose logs -f xgateway

# Last 50 lines from a specific service
docker compose logs --tail=50 giveaway
```

If any container shows `Exit` or `Restarting`, check its logs immediately — the most common causes are a missing env variable or a failed database connection.

---

## 7. Phase 5 — Expose XGateway via Nginx + SSL

At this point, `xgateway` is running on `localhost:8081` inside the Droplet. This phase makes it publicly accessible over HTTPS.

### Step 5.1 — Point Your Domain to the Droplet

In your DNS provider (Cloudflare, Namecheap, etc.), create an **A record**:

| Field | Value                   |
| ----- | ----------------------- |
| Name  | `api` (or `@` for root) |
| Type  | `A`                     |
| Value | `YOUR_DROPLET_IP`       |
| TTL   | `300`                   |

DNS propagation can take a few minutes up to 24 hours. You can verify it:

```bash
# From your local machine
dig api.yourdomain.com +short
# Should return your Droplet IP
```

### Step 5.2 — Install Nginx and Certbot

On the Droplet:

```bash
apt update
apt install -y nginx certbot python3-certbot-nginx
```

### Step 5.3 — Create the Nginx Config

```bash
nano /etc/nginx/sites-available/instantf-botx
```

Paste:

```nginx
server {
    listen 80;
    server_name api.yourdomain.com;

    location / {
        proxy_pass         http://localhost:8081;
        proxy_http_version 1.1;

        proxy_set_header   Upgrade           $http_upgrade;
        proxy_set_header   Connection        'upgrade';
        proxy_set_header   Host              $host;
        proxy_set_header   X-Real-IP         $remote_addr;
        proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;

        proxy_cache_bypass $http_upgrade;

        # Increase timeouts for long-running requests
        proxy_connect_timeout 60s;
        proxy_send_timeout    60s;
        proxy_read_timeout    60s;
    }
}
```

Enable the site and test the config:

```bash
ln -s /etc/nginx/sites-available/instantf-botx /etc/nginx/sites-enabled/
nginx -t
# Expected: syntax is ok / test is successful
systemctl reload nginx
```

### Step 5.4 — Issue an SSL Certificate

```bash
certbot --nginx -d api.yourdomain.com
```

Follow the interactive prompts:

- Enter your email for renewal notifications
- Agree to the Terms of Service
- Choose whether to redirect HTTP to HTTPS (recommended: **yes**)

Certbot will automatically modify your Nginx config to handle HTTPS and configure auto-renewal via a systemd timer.

Verify auto-renewal works:

```bash
certbot renew --dry-run
# Should complete without errors
```

Your API is now live at `https://api.yourdomain.com` 🎉

---

## 8. Phase 6 — Run Database Migrations

> Run this **from your local machine**, not the Droplet.

### Install `golang-migrate` (if not already installed)

**macOS:**

```bash
brew install golang-migrate
```

**Linux:**

```bash
curl -L https://github.com/golang-migrate/migrate/releases/download/v4.17.0/migrate.linux-amd64.tar.gz | tar xvz
sudo mv migrate /usr/local/bin/
migrate --version
```

### Run the migrations

```bash
migrate \
  -path ./migrations \
  -database "postgresql://botx_user:YOUR_PASSWORD@your-cluster.cockroachlabs.cloud:26257/instantf_bot_x?sslmode=verify-full" \
  up
```

Expected output — each migration file applied in sequence:

```
1/u create_users_table (12.345ms)
2/u create_wallets_table (8.123ms)
3/u create_transactions_table (9.456ms)
...
```

If a migration fails partway through, fix the issue and run:

```bash
# Check current migration version
migrate \
  -path ./migrations \
  -database "postgresql://..." \
  version

# Roll back the last migration if needed
migrate \
  -path ./migrations \
  -database "postgresql://..." \
  down 1
```

Once all migrations complete without errors, your database schema is ready and services can begin handling live traffic.

---

## 9. Post-Deployment Operations

### Common Commands

```bash
# View status of all containers
docker compose ps

# Stream live logs from all services
docker compose logs -f

# Stream logs from one service
docker compose logs -f xgateway

# Restart a single service (e.g., after a config change)
docker compose restart notification

# Stop all services (containers removed, images kept)
docker compose down

# Stop and remove volumes (⚠️ destructive — only if needed)
docker compose down -v
```

### Deploying Updates

When you push new code and want to redeploy:

```bash
cd /opt/bot-x

# Pull latest code
git pull origin main

# Rebuild images and restart services with zero-downtime rolling update
docker compose up -d --build
```

Docker Compose will only rebuild and restart containers whose images have changed.

### Monitoring Resource Usage

```bash
# Live CPU/RAM usage per container
docker stats

# Disk usage by Docker images and volumes
docker system df
```

### Cleaning Up Old Images

After several deployments, unused images can accumulate and fill disk space:

```bash
# Remove dangling (unused) images
docker image prune -f

# Remove all unused images, containers, networks (safe if all services are running)
docker system prune -f
```

---

## 10. Troubleshooting

### A container keeps restarting

```bash
docker compose logs --tail=100 SERVICE_NAME
```

Common causes:

- **Missing env variable** — check your `.env` file for typos or missing values
- **Cannot connect to database** — verify `DATABASE_URL` is correct and CockroachDB allows your Droplet's IP (check CockroachDB's network allowlist)
- **Port conflict** — ensure nothing else is using port `8081` on the host (`ss -tlnp | grep 8081`)

### Cannot connect to CockroachDB

CockroachDB Serverless/Dedicated restricts connections by IP by default. Add your Droplet's IP to the allowlist:

1. CockroachDB dashboard → your cluster → **Networking** → **Add IP**
2. Enter your Droplet's public IP address
3. Save and retry

### SSL certificate fails

```bash
# Check Nginx is running and config is valid
systemctl status nginx
nginx -t

# Ensure port 80 is open for Let's Encrypt HTTP challenge
ufw allow 80
ufw allow 443
```

### gRPC services cannot reach each other

Verify all services are on the same Docker network:

```bash
docker network ls
docker network inspect bot-x_default
```

All 10 services should appear in the `Containers` section. If a service is missing, check its `docker-compose.yml` definition.

### Check the RSA key is being parsed correctly

If SafeHaven-related services are failing to start, the PEM key likely has formatting issues. Verify the key in your `.env` has real newlines (not `\n` escape sequences):

```bash
grep -A 3 "SAFEHAVEN_PRIVATE_KEY_PEM" .env
# The key content should be on separate lines, not a single long string
```

---

## Deployment Checklist

Use this before going live:

- [x] CockroachDB cluster created with `botx_user` and `instantf_bot_x` database
- [x] Valkey cluster provisioned on DigitalOcean
- [x] Droplet created from Docker Marketplace image (4GB RAM+)
- [x] SSH access confirmed
- [x] Repository cloned to `/opt/bot-x`
- [x] `.env` file created with all secrets, permissions set to `600`
- [x] `docker-compose.yml` updated — local infra services removed
- [x] `docker compose up -d --build` completed with all 10 services `Up`
- [ ] DNS A record pointing to Droplet IP
- [ ] Nginx installed and config tested (`nginx -t`)
- [ ] SSL certificate issued via Certbot
- [ ] HTTPS endpoint responding at `https://api.yourdomain.com`
- [ ] Database migrations run successfully
- [ ] X/Twitter webhooks enabled

---

_Generated for InstantF Bot-X — Production Deployment (Option 2: Droplet + Docker Compose)_
