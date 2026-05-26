# Deployment Guide: DigitalOcean

The InstantF Bot-X microservices architecture is fully Dockerized using minimal-attack-surface `gcr.io/distroless/static` images.

Depending on your budget and scaling needs, there are two primary ways to deploy this on DigitalOcean: **DigitalOcean App Platform** (managed, easiest) or a **Docker Compose Droplet** (most cost-effective for 10 services).

## Prerequisites

1. [doctl CLI](https://docs.digitalocean.com/reference/doctl/how-to/install/) installed and authenticated (`doctl auth init`).
2. Docker installed locally.
3. Managed PostgreSQL and Redis clusters (DigitalOcean Managed Databases recommended).
4. HashiCorp Vault instance for production secrets management.

## Deployment Readiness Checklist

Before deploying, verify the following:

1. `DATABASE_URL` points to your managed PostgreSQL instance.
2. `REDIS_URL` points to your managed Redis instance.
3. Vault is reachable from the target environment and contains the required secrets.
4. The repo branch includes the int64 monetary migration and protobuf regen.
5. Docker Compose can start the one-shot `migrate` service successfully before app services boot.

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
        value: giveaway:50052 # Internal App Platform routing
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

_Note: App Platform bills per component. Running 10 separate services can get expensive. If cost is a major concern, see Option 2._

---

## Option 2: Docker Compose on a Dedicated Droplet (Most Cost-Effective)

Since gRPC communication between 10 microservices is complex, running them on a single high-CPU Droplet via `docker-compose` is highly efficient and eliminates network latency between services.

### 1. Provision a Droplet

Create a new Droplet using the **Docker 1-Click App** from the DigitalOcean Marketplace (Ubuntu with Docker & Compose pre-installed). Recommend at least 4GB RAM / 2 CPUs.

### 2. Update `docker-compose.yml` for Production

The provided `docker-compose.yml` now includes a one-shot `migrate` service that applies the SQL migrations before the database-backed services start. For production use, you should:

1. Point `DATABASE_URL` to your Managed PostgreSQL connection string and `REDIS_URL` to your Managed Redis endpoint.
2. Ensure internal gRPC communication uses Docker's internal DNS network aliases (e.g., `GRPC_GIVEAWAY_ADDR=giveaway:50052`).
3. Keep Vault reachable from the deployment target and seed the required secrets before boot.

### 3. Deploy

1. SSH into your Droplet.
2. Clone your repository.
3. Create a `.env` file containing your production secrets (Vault tokens, Managed DB URLs).
4. Build and start the services. The `migrate` container will run first and the app services will wait for it to complete successfully:

```bash
docker compose -f docker-compose.yml up -d --build
```

If you want to validate only the migration job first, run:

```bash
docker compose run --rm migrate
```

### 4. Reverse Proxy / SSL

To expose the `xgateway` webhook securely, install Nginx or Caddy on the Droplet and set up a reverse proxy to route external port 443 (HTTPS) to the `xgateway` container's exposed port `8081`.

### 5. Post-Deploy Smoke Tests

After deployment, verify:

1. `docker compose ps` shows the `migrate` service exited successfully and the app services are healthy.
2. `xgateway` responds on the public webhook endpoint.
3. `giveaway`, `entry`, and `payment-router` can resolve each other by Compose service name.
4. A test giveaway can be created end-to-end using the migrated `BIGINT` monetary fields.

---

## Security Notes

- **Distroless Images:** The images use `gcr.io/distroless/static:nonroot`. They do not contain a shell (`/bin/sh` or `/bin/bash`). You cannot use `docker exec -it <container> sh`. This significantly hardens the containers against remote code execution.
- **Fail-Closed Private Keys:** The `payment-router`, `reconciliation`, and `kyc` services will intentionally crash on boot if they cannot read the SafeHaven private key from Vault or the file system in production. Ensure Vault is correctly seeded before deploying these services.
