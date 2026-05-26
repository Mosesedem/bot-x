# Deployment Guide: DigitalOcean

The InstantF Bot-X microservices architecture is fully Dockerized using minimal-attack-surface `gcr.io/distroless/static` images. 

Depending on your budget and scaling needs, there are two primary ways to deploy this on DigitalOcean: **DigitalOcean App Platform** (managed, easiest) or a **Docker Compose Droplet** (most cost-effective for 10 services).

## Prerequisites
1. [doctl CLI](https://docs.digitalocean.com/reference/doctl/how-to/install/) installed and authenticated (`doctl auth init`).
2. Docker installed locally.
3. Managed PostgreSQL and Redis clusters (DigitalOcean Managed Databases recommended).
4. HashiCorp Vault instance for production secrets management.

---

## Option 1: DigitalOcean App Platform (Recommended for Managed Scaling)

DigitalOcean App Platform is a Platform-as-a-Service (PaaS) similar to Heroku. You can deploy all 10 microservices within a single "App" using an App Spec, allowing them to communicate securely over internal routing.

### 1. Create the App Spec (`do-app.yaml`)
Create a file named `do-app.yaml` at the root of your project. This defines all your services, environment variables, and how they build from your Dockerfiles.

```yaml
name: instantf-bot-x
region: lon1
services:
  # The publicly accessible webhook gateway
  - name: xgateway
    dockerfile_path: services/xgateway/Dockerfile
    source_dir: .
    http_port: 8080
    envs:
      - key: APP_ENV
        value: production
      - key: DATABASE_URL
        value: ${db.DATABASE_URL}
      - key: GRPC_GIVEAWAY_ADDR
        value: giveaway:50052  # Internal App Platform routing
    routes:
      - path: /

  # Internal gRPC Microservice (No public routes)
  - name: giveaway
    dockerfile_path: services/giveaway/Dockerfile
    source_dir: .
    internal_ports: [50052]
    envs:
      - key: APP_ENV
        value: production
      - key: DATABASE_URL
        value: ${db.DATABASE_URL}

  # Add definitions for entry, payment-router, kyc, compliance, audit, notification, reconciliation, admin
```

### 2. Deploy the App
```bash
doctl apps create --spec do-app.yaml
```

*Note: App Platform bills per component. Running 10 separate services can get expensive. If cost is a major concern, see Option 2.*

---

## Option 2: Docker Compose on a Dedicated Droplet (Most Cost-Effective)

Since gRPC communication between 10 microservices is complex, running them on a single high-CPU Droplet via `docker-compose` is highly efficient and eliminates network latency between services.

### 1. Provision a Droplet
Create a new Droplet using the **Docker 1-Click App** from the DigitalOcean Marketplace (Ubuntu with Docker & Compose pre-installed). Recommend at least 4GB RAM / 2 CPUs.

### 2. Update `docker-compose.yml` for Production
The provided `docker-compose.yml` is mostly ready, but you should:
1. Remove the local `postgres`, `redis`, and `vault` infrastructure blocks and replace their URLs with your Managed DigitalOcean Database connection strings.
2. Ensure internal gRPC communication uses Docker's internal DNS network aliases (e.g., `GRPC_GIVEAWAY_ADDR=giveaway:50052`).

### 3. Deploy
1. SSH into your Droplet.
2. Clone your repository.
3. Create a `.env` file containing your production secrets (Vault tokens, Managed DB URLs).
4. Build and start the services:

```bash
docker compose -f docker-compose.yml up -d --build
```

### 4. Reverse Proxy / SSL
To expose the `xgateway` webhook securely, install Nginx or Caddy on the Droplet and set up a reverse proxy to route external port 443 (HTTPS) to the `xgateway` container's exposed port `8081`. 

---

## Security Notes
- **Distroless Images:** The images use `gcr.io/distroless/static:nonroot`. They do not contain a shell (`/bin/sh` or `/bin/bash`). You cannot use `docker exec -it <container> sh`. This significantly hardens the containers against remote code execution.
- **Fail-Closed Private Keys:** The `payment-router`, `reconciliation`, and `kyc` services will intentionally crash on boot if they cannot read the SafeHaven private key from Vault or the file system in production. Ensure Vault is correctly seeded before deploying these services.
