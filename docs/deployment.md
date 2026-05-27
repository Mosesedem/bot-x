# Bot-X Deployment Guide: Digital Ocean Docker Droplet

**Production-ready deployment** for Bot-X using Docker Compose on a Digital Ocean Ubuntu Droplet.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│  Digital Ocean Droplet (Ubuntu 22.04 LTS)                   │
│  4GB RAM / 2 vCPUs / 80GB SSD minimum                        │
│                                                               │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Nginx (SSL termination, reverse proxy)              │   │
│  │  Ports: 80, 443 → Proxies to xgateway:8080           │   │
│  └─────────────────────────────────────────────────────┘   │
│                          │                                  │
│  ┌─────────────────────────────────────────────────────┐   │
│  │  Docker Network: bot-x_default                       │   │
│  │                                                       │   │
│  │  ┌─────────┐ ┌──────────┐ ┌──────────────┐           │   │
│  │  │xgateway │ │ giveaway │ │payment-router│ ...       │   │
│  │  │ :8080   │ │ :50052   │ │ :50054       │           │   │
│  │  └────┬────┘ └──────────┘ └──────────────┘           │   │
│  │       │                                              │   │
│  │  ┌────▼────────────────────────────────────────┐  │   │
│  │  │  Data Stores (Containerized)                   │  │   │
│  │  │  • Postgres 15  → Port 5432 (localhost only)   │  │   │
│  │  │  • Redis 7      → Port 6379 (localhost only)    │  │   │
│  │  │  • ClickHouse  → Ports 9000, 8123 (localhost)  │  │   │
│  │  └────────────────────────────────────────────────┘  │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

## Prerequisites

- Domain name (e.g., `api.yourdomain.com`)
- Digital Ocean account
- SSH key pair

## Phase 1: Provision Droplet

### 1. Create Droplet

1. Digital Ocean Dashboard → **Create** → **Droplets**
2. **Choose an image:** Marketplace tab → **Docker** on Ubuntu 22.04 LTS
3. **Plan:** Basic, 4GB RAM / 2 vCPUs ($24/month minimum)
4. **Region:** Choose closest to your users (e.g., London, NYC, Frankfurt)
5. **Authentication:** Add your SSH public key
6. **Hostname:** `bot-x-prod`
7. Click **Create Droplet**

### 2. Configure DNS

In your DNS provider, add an A record:

```
Type: A
Name: api (or @ for root)
Value: YOUR_DROPLET_IP
TTL: 300
```

### 3. SSH to Droplet

```bash
ssh root@YOUR_DROPLET_IP

# Verify Docker
docker --version
docker-compose --version
```

## Phase 2: Initial Server Setup

### 1. Create Deploy User (Recommended)

```bash
# On the droplet
adduser botx
usermod -aG sudo botx
usermod -aG docker botx

# Copy SSH key
mkdir -p /home/botx/.ssh
cp ~/.ssh/authorized_keys /home/botx/.ssh/
chown -R botx:botx /home/botx/.ssh
chmod 700 /home/botx/.ssh
chmod 600 /home/botx/.ssh/authorized_keys
```

Logout and reconnect as `botx` user:

```bash
ssh botx@YOUR_DROPLET_IP
```

### 2. Create Application Directory

```bash
sudo mkdir -p /opt/bot-x
sudo chown botx:botx /opt/bot-x
cd /opt/bot-x
```

### 3. Clone Repository

```bash
git clone https://github.com/Mosesedem/bot-x.git .
```

Or if private repo, use deploy key:

```bash
# Generate deploy key
ssh-keygen -t ed25519 -C "botx-deploy" -f ~/.ssh/deploy_key -N ""
cat ~/.ssh/deploy_key.pub
# Add to GitHub repo → Settings → Deploy Keys

# Configure SSH to use deploy key
cat >> ~/.ssh/config << 'EOF'
Host github.com
  IdentityFile ~/.ssh/deploy_key
  IdentitiesOnly yes
EOF

git clone git@github.com:Mosesedem/bot-x.git /opt/bot-x
```

## Phase 3: Configure Environment

### 1. Create Production Environment File

```bash
cd /opt/bot-x
cp .env.production.example .env
nano .env
```

**Critical settings to change:**

```bash
# Database (generate strong passwords)
POSTGRES_PASSWORD=your_very_strong_random_password
CLICKHOUSE_PASSWORD=your_very_strong_random_password

# Domain
BASE_URL=https://api.yourdomain.com

# Twitter API (get from developer portal)
X_CONSUMER_KEY=your_actual_consumer_key
X_CONSUMER_SECRET=your_actual_consumer_secret
X_ACCESS_TOKEN=your_actual_access_token
X_ACCESS_SECRET=your_actual_access_secret
X_BEARER_TOKEN=your_actual_bearer_token
X_WEBHOOK_SECRET=generate_random_32_char_string
BOT_TWITTER_ID=your_bot_account_numeric_id

# Safe Haven (for Nigerian payouts)
SAFEHAVEN_CLIENT_ID=your_safehaven_id
SAFEHAVEN_CLIENT_SECRET=your_safehaven_secret
# IMPORTANT: Paste full PEM with literal newlines, NOT \n escapes
SAFEHAVEN_PRIVATE_KEY_PEM="-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEA...
...
-----END RSA PRIVATE KEY-----"

# Stripe (for US/international)
STRIPE_SECRET_KEY=sk_live_...

# Admin JWT (generate strong secret)
ADMIN_JWT_SECRET=your_min_32_char_random_string
```

**Secure the file:**

```bash
chmod 600 .env
```

## Phase 4: Deploy Application

### Option A: Using Deploy Script (Recommended)

```bash
cd /opt/bot-x
make deploy-prod
```

### Option B: Manual Deployment

```bash
cd /opt/bot-x

# Build and start
docker-compose -f docker-compose.prod.yml up -d --build

# Run migrations
docker-compose -f docker-compose.prod.yml run --rm migrate

# Check status
docker-compose -f docker-compose.prod.yml ps
```

## Phase 5: Configure Nginx + SSL

### 1. Install Nginx and Certbot

```bash
sudo apt update
sudo apt install -y nginx certbot python3-certbot-nginx
```

### 2. Configure Nginx

```bash
sudo cp /opt/bot-x/nginx/bot-x.conf /etc/nginx/sites-available/bot-x

# Edit domain
sudo nano /etc/nginx/sites-available/bot-x
# Change: server_name api.yourdomain.com;

# Enable site
sudo ln -sf /etc/nginx/sites-available/bot-x /etc/nginx/sites-enabled/
sudo rm -f /etc/nginx/sites-enabled/default

# Test config
sudo nginx -t
sudo systemctl restart nginx
```

### 3. Obtain SSL Certificate

```bash
sudo certbot --nginx -d api.yourdomain.com

# Follow prompts:
# - Enter email
# - Agree to terms
# - Choose to redirect HTTP to HTTPS (yes)

# Verify auto-renewal
sudo certbot renew --dry-run
```

### 4. Configure Firewall

```bash
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow ssh
sudo ufw allow 'Nginx Full'
sudo ufw enable
```

## Phase 6: Configure Twitter Webhook

### 1. Verify Endpoint is Reachable

```bash
curl https://api.yourdomain.com/health
# Should return 200 OK
```

### 2. Register Webhook with Twitter

Use Twitter API or dashboard to register your webhook URL:

```
Webhook URL: https://api.yourdomain.com/webhook/twitter
```

The CRC (Challenge-Response Check) will verify your endpoint.

## Phase 7: Post-Deployment Operations

### Daily Operations

```bash
# View all service logs
cd /opt/bot-x && make prod-logs

# View specific service logs
docker-compose -f docker-compose.prod.yml logs -f xgateway

# Check service status
make prod-ps

# Restart a service
docker-compose -f docker-compose.prod.yml restart payment-router
```

### Database Backups

```bash
# Manual backup
cd /opt/bot-x
make prod-backup
# Creates: backup-YYYYMMDD-HHMMSS.sql

# Automated daily backup (add to crontab)
(crontab -l 2>/dev/null; echo "0 2 * * * cd /opt/bot-x && make prod-backup >> backup.log 2>&1") | crontab -
```

### Updates and Redeployments

```bash
cd /opt/bot-x

# Pull latest code
git pull origin main

# Redeploy (zero-downtime with compose)
make deploy-prod

# Or manually:
docker-compose -f docker-compose.prod.yml up -d --build
```

### Monitoring Resource Usage

```bash
# Container stats
docker stats

# Disk usage
docker system df

# Clean up old images
docker image prune -f
docker system prune -f
```

## Troubleshooting

### Service Won't Start

```bash
# Check logs
docker-compose -f docker-compose.prod.yml logs --tail=100 SERVICE_NAME

# Common issues:
# 1. Missing env var - check .env file
# 2. Database not ready - wait for postgres to be healthy
# 3. Port conflict - check `ss -tlnp | grep PORT`
```

### Database Connection Issues

```bash
# Test database from container
docker-compose -f docker-compose.prod.yml exec postgres pg_isready

# Check env vars are loaded
docker-compose -f docker-compose.prod.yml exec xgateway env | grep DATABASE
```

### SSL Certificate Issues

```bash
# Check certbot logs
sudo journalctl -u certbot

# Manually renew
sudo certbot renew
```

### Webhook Not Receiving Events

```bash
# Check xgateway is receiving requests
docker-compose -f docker-compose.prod.yml logs -f xgateway | grep webhook

# Verify Twitter webhook registration
curl -H "Authorization: Bearer $X_BEARER_TOKEN" \
  https://api.twitter.com/1.1/account_activity/webhooks.json
```

## Security Hardening Checklist

- [ ] Non-root deploy user created
- [ ] `.env` file has `chmod 600`
- [ ] Firewall enabled (UFW), only 22, 80, 443 open
- [ ] SSL certificate installed and auto-renewal configured
- [ ] Admin service only accessible via localhost (SSH tunnel)
- [ ] Database ports bound to 127.0.0.1 only
- [ ] Twitter webhook signature verification enabled
- [ ] Strong passwords for all services
- [ ] Regular backups configured

## Scaling Considerations

When traffic grows:

1. **Vertical Scaling:** Resize droplet to 8GB RAM / 4 vCPUs
2. **Database:** Migrate to Digital Ocean Managed PostgreSQL
3. **Redis:** Migrate to Digital Ocean Managed Redis
4. **Horizontal:** Consider Kubernetes (DOKS) when 1 droplet isn't enough

## Emergency Procedures

### Full System Recovery

```bash
# If everything is broken, reset:
cd /opt/bot-x
docker-compose -f docker-compose.prod.yml down -v  # WARNING: Destroys data
# Restore from backup:
docker-compose -f docker-compose.prod.yml up -d postgres
cat backup-YYYYMMDD.sql | docker-compose -f docker-compose.prod.yml exec -T postgres psql -U botx -d instantf_bot_x
```

### Rollback to Previous Version

```bash
cd /opt/bot-x
git log --oneline -10  # Find previous commit
git checkout PREVIOUS_COMMIT
docker-compose -f docker-compose.prod.yml up -d --build
```
