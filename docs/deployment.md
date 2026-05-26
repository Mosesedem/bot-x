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

### 1.2 Cache/PubSub: Redis
You can use DigitalOcean's Managed Redis for this.
1. In the DigitalOcean console, go to **Databases** -> **Create Database Cluster**.
2. Select **Redis**.
3. Choose a node plan (e.g., 1GB RAM) and deploy it to the same region where your apps will live (e.g., `lon1` or `nyc1`).
4. Once provisioned, copy the **Connection String** (use the "Rediss" TLS URL).

### 1.3 Secrets Management: HashiCorp Vault
Vault is critical for this architecture as it stores API keys (Twitter, Stripe, SafeHaven) and private RSA keys.

**Option A: HCP Vault Secrets (Recommended, Easiest)**
The easiest path is to use HashiCorp's fully managed cloud service.
1. Sign up for [HCP Vault Secrets](https://cloud.hashicorp.com/).
2. Create an App named `instantf-bot-x`.
3. Generate a Service Principal Token. This becomes your `VAULT_TOKEN`.

**Option B: Self-Hosted Vault on a DigitalOcean Droplet**
If you prefer to host Vault yourself:
1. Spin up a basic Ubuntu Droplet.
2. Install Vault:
   ```bash
   sudo apt update && sudo apt install gpg wget
   wget -O- https://apt.releases.hashicorp.com/gpg | sudo gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
   echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/hashicorp.list
   sudo apt update && sudo apt install vault
   ```
3. Configure `/etc/vault.d/vault.hcl` to use file storage (for simplicity) or Consul, and enable TLS.
4. Start Vault: `sudo systemctl start vault`
5. Initialize Vault: `vault operator init` (Save the unseal keys and Root Token!)
6. Unseal Vault: `vault operator unseal` (Do this 3 times using the keys).
7. Log in: `vault login <Root_Token>`

#### Seeding Vault with Secrets
Whether using HCP or Self-hosted, you must enable the KV engine and seed your secrets. The microservices expect secrets to be stored at specific paths.

```bash
# Enable Key-Value V2 secrets engine at path secret/
vault secrets enable -path=secret kv-v2

# 1. X/Twitter API Secrets
vault kv put secret/twitter api_key="YOUR_API_KEY" api_secret="YOUR_API_SECRET" bearer_token="YOUR_BEARER" client_id="YOUR_CLIENT_ID" client_secret="YOUR_CLIENT_SECRET" webhook_env="YOUR_WEBHOOK_ENV"

# 2. Stripe Secrets
vault kv put secret/stripe secret_key="YOUR_STRIPE_SECRET" webhook_secret="YOUR_STRIPE_WEBHOOK_SECRET"

# 3. SafeHaven RSA Private Key (Must be PEM formatted)
# The payment-router, kyc, and reconciliation services will CRASH on boot if this is missing!
vault kv put secret/safehaven private_key=@safehaven_private.pem
```

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
        value: "rediss://default:pass@...:25061" # DO Redis URL
      - key: VAULT_ADDR
        value: "https://your-vault-url.com"
      - key: VAULT_TOKEN
        value: "your-vault-token"
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
      - key: VAULT_ADDR
        value: "https://your-vault-url.com"
      - key: VAULT_TOKEN
        value: "your-vault-token"

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
   VAULT_ADDR=https://your-vault-url.com
   VAULT_TOKEN=your-vault-token
   
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
