# Production Deployment Guide: InstantF Bot-X

This guide covers the complete, end-to-end production deployment of the 10 InstantF Bot-X microservices on **DigitalOcean**, alongside the external infrastructure you will need: **CockroachDB**, **Redis**, and **HashiCorp Vault**.

---

## 1. Infrastructure Setup

Before deploying the microservices, you must provision the backing infrastructure.

### 1.1 Database: CockroachDB
Since you are using CockroachDB, it is recommended to use **CockroachDB Dedicated** or **CockroachDB Serverless** rather than hosting it manually, to ensure high availability and distributed replication.

1. Create a cluster on [CockroachLabs](https://cockroachlabs.cloud/).
2. Create a SQL user (e.g., `botx_user`) and generate a password.
3. Create a database named `instantf_bot_x`.
4. Copy the connection string. It will look like this:
   `postgresql://botx_user:password@free-tier.gcp-us-central1.cockroachlabs.cloud:26257/instantf_bot_x?sslmode=verify-full`

### 1.2 Cache/PubSub: Valkey (Redis Alternative)
DigitalOcean has transitioned their managed offering to **Valkey**, which is an open-source, 100% drop-in replacement for Redis. Our Go services use standard Redis clients that work perfectly with Valkey.
1. In the DigitalOcean console, go to **Databases** -> **Create Database Cluster**.
2. Select **Valkey** (or Redis, if the old name is still visible).
3. Choose a node plan (e.g., 1GB RAM) and deploy it to the same region where your apps will live.
4. Once provisioned, copy the **Connection String**. It functions identically to a `rediss://` URL and can be plugged directly into the `REDIS_URL` variable.

### 1.3 Secrets Management: Environment Variables
InstantF Bot-X relies on standard environment variables for configuration. You do not need external secrets managers like Vault. All secrets, including multiline RSA keys, can be passed directly as environment variables or via a `.env` file on your Droplet.

For the SafeHaven RSA Private Key, use the `SAFEHAVEN_PRIVATE_KEY_PEM` variable. Paste the exact, multiline raw string of your `.pem` key directly into this variable on DigitalOcean.

---

## 2. Deploying the Microservices on DigitalOcean

There are two main options for hosting the 10 Go microservices. Option 1 (App Platform) is fully managed. Option 2 (Droplet with Compose) is much cheaper.

### Option 1: DigitalOcean App Platform (PaaS)
This option uses a single `do-app.yaml` file to deploy all 10 services. DigitalOcean builds the Dockerfiles, assigns internal DNS names for gRPC communication, and scales the apps.

1. Ensure the `doctl` CLI is installed and authenticated.
2. Create a `do-app.yaml` file locally (see snippet below).
3. Deploy the spec:
   ```bash
   doctl apps create --spec do-app.yaml
   ```

**App Spec Template snippet (`do-app.yaml`):**
```yaml
name: instantf-bot-x
region: lon1
services:
  # The Public Gateway
  - name: xgateway
    dockerfile_path: services/xgateway/Dockerfile
    source_dir: .
    http_port: 8080
    envs:
      - key: APP_ENV
        value: production
      - key: DATABASE_URL
        value: "postgresql://botx_user:pass@...:26257/instantf_bot_x" # CockroachDB URL
      - key: REDIS_URL
        value: "rediss://default:pass@...:25061" # DO Valkey URL
      - key: SAFEHAVEN_PRIVATE_KEY_PEM
        value: "-----BEGIN RSA PRIVATE KEY-----\n..."
      # Internal gRPC routing (Format: app_name:port)
      - key: GRPC_GIVEAWAY_ADDR
        value: giveaway:50052  
      - key: GRPC_PAYMENT_ROUTER_ADDR
        value: payment-router:50053
    routes:
      - path: /

  # Internal gRPC Microservice
  - name: giveaway
    dockerfile_path: services/giveaway/Dockerfile
    source_dir: .
    internal_ports: [50052] # No public internet access
    envs:
      - key: APP_ENV
        value: production
      - key: DATABASE_URL
        value: "postgresql://botx_user:pass@...:26257/instantf_bot_x"

  # [Repeat for entry, payment-router, kyc, compliance, audit, notification, reconciliation, admin]
```

---

### Option 2: Docker Compose on a Dedicated Droplet (IaaS)

If running 10 apps on the App Platform is too expensive, you can run them all on a single high-CPU Droplet using Docker Compose.

1. **Provision a Droplet:** Go to DigitalOcean -> Create Droplet. Go to **Marketplace** and select the **Docker** image (Ubuntu with Docker pre-installed). Recommend at least 4GB RAM / 2 CPUs.
2. **SSH into the Droplet** and clone your repository.
3. **Configure the Environment:**
   Create a `.env` file at the root of the project with your production variables:
   ```env
   APP_ENV=production
   DATABASE_URL=postgresql://botx_user:password@host:26257/instantf_bot_x?sslmode=verify-full
   REDIS_URL=rediss://default:password@host:25061
   SAFEHAVEN_PRIVATE_KEY_PEM="-----BEGIN RSA PRIVATE KEY-----\n..."
   STRIPE_SECRET_KEY=sk_live_...
   
   # Internal Docker Network gRPC mapping
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

4. **Update `docker-compose.yml`:**
   Modify your existing `docker-compose.yml` to remove the local `postgres`, `redis`, and `vault` services, since you are now using managed infrastructure. Ensure all 10 Go services (`xgateway`, `giveaway`, etc.) are pointing to their respective `Dockerfile`s.

5. **Deploy:**
   Run the following command to build the distroless images and start the services in the background:
   ```bash
   docker compose up -d --build
   ```

6. **Expose XGateway (Nginx / SSL):**
   Install Caddy or Nginx on the Droplet to act as a reverse proxy, routing incoming port `443` (HTTPS) traffic from your domain to `localhost:8081` (where the `xgateway` container is exposed).

---

## 3. Post-Deployment: Migrations

Before turning on X/Twitter webhooks, you must run your database migrations against the new CockroachDB cluster.

From your local machine (with `golang-migrate` installed):
```bash
migrate -path migrations -database "postgresql://botx_user:password@host:26257/instantf_bot_x?sslmode=verify-full" up
```

Once migrations are complete, your microservices architecture is live and ready to receive traffic.
