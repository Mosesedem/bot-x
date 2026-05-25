# Human Tasks & Deployment Guide for instantf-bot-x

This document provides a comprehensive step-by-step guide for developers and DevOps engineers to configure, run, and deploy the `instantf-bot-x` microservices architecture.

---

## 1. Local Prerequisites
Ensure the following tools are installed on your local machine:
- **Go 1.22+**: For compiling the backend services.
- **Docker & Docker Compose**: For running local infrastructure (Redis, Postgres, ClickHouse, Vault).
- **Buf**: For compiling Protobuf definitions (`brew install bufbuild/buf/buf` on macOS).
- **Migrate CLI**: For executing database migrations (`brew install golang-migrate` on macOS).
- **Heroku CLI**: If you plan on deploying to Heroku (`brew tap heroku/brew && brew install heroku`).

## 2. Environment Configuration
Create a `.env` file in the root of the repository. Below is a template of the required environment variables:
```env
# Infrastructure
DATABASE_URL=postgres://postgres:postgres@localhost:5432/botx?sslmode=disable
REDIS_URL=localhost:6379
CLICKHOUSE_URL=localhost:9000
CLICKHOUSE_DB=default
VAULT_ADDR=http://127.0.0.1:8200
VAULT_TOKEN=dev-root-token

# X (Twitter) API
X_CONSUMER_KEY=your_consumer_key
X_CONSUMER_SECRET=your_consumer_secret
X_ACCESS_TOKEN=your_access_token
X_ACCESS_SECRET=your_access_secret
X_WEBHOOK_ENV=dev
X_BEARER_TOKEN=your_bearer_token
X_WEBHOOK_SECRET=your_webhook_secret
BOT_TWITTER_ID=123456789
BOT_TWITTER_HANDLE=instantf

# Payment Gateways
SAFEHAVEN_BASE_URL=https://api.safehaven.com
SAFEHAVEN_CLIENT_ID=client_id
SAFEHAVEN_CLIENT_SECRET=client_secret

# gRPC Addresses (for local dev)
GRPC_XGATEWAY_ADDR=localhost:50051
GRPC_GIVEAWAY_ADDR=localhost:50052
GRPC_ENTRY_ADDR=localhost:50053
GRPC_PAYMENT_ROUTER_ADDR=localhost:50054
GRPC_KYC_ADDR=localhost:50055
GRPC_COMPLIANCE_ADDR=localhost:50056
GRPC_AUDIT_ADDR=localhost:50057
GRPC_NOTIFICATION_ADDR=localhost:50058
GRPC_RECONCILIATION_ADDR=localhost:50059
```

## 3. Generating Protobuf Stubs
If you make changes to the `.proto` files in the `/proto` directory, you must regenerate the Go stubs:
```bash
cd proto
buf generate
```
The generated files will be placed in `gen/go`, which is configured as its own module in `go.work`.

## 4. Running Infrastructure Locally
Start the backing services using Docker Compose:
```bash
docker-compose up -d postgres redis clickhouse vault
```

## 5. Building and Running Services Locally
To build all microservices at once, run:
```bash
for dir in services/*; do 
  svc=$(basename "$dir")
  echo "Building $svc..."
  go build -o "bin/$svc" "./services/$svc/cmd/main.go"
done
```
To run a specific service (e.g., `xgateway`):
```bash
./bin/xgateway
```

---

## 6. Deployment Guide: Heroku
Because `instantf-bot-x` is a monorepo consisting of 10 microservices, the most efficient way to deploy it to Heroku is via **Heroku Container Registry** using the generated Dockerfiles.

### Step 1: Login to Heroku
```bash
heroku login
heroku container:login
```

### Step 2: Create Heroku Apps
Create a separate Heroku app for each microservice (Heroku only supports one web process port per app natively).
```bash
heroku create instantf-xgateway
heroku create instantf-giveaway
# Repeat for all 10 services...
```

### Step 3: Build and Push Docker Images
Heroku requires the image to be pushed to their registry. From the root of the repository, execute the following for each service. Note the `-f` flag pointing to the service-specific Dockerfile:

```bash
# Example for xgateway
heroku container:push web -a instantf-xgateway -f services/xgateway/Dockerfile .
```

### Step 4: Release the Containers
Once the image is pushed, release it so it starts running:
```bash
heroku container:release web -a instantf-xgateway
```

### Step 5: Configure Environment Variables on Heroku
Set the necessary configuration variables for each Heroku app to ensure they can communicate. Note that you must provide the internal Heroku URLs (or private space URLs) for the `GRPC_*_ADDR` variables.
```bash
heroku config:set DATABASE_URL="postgres://..." -a instantf-xgateway
heroku config:set GRPC_GIVEAWAY_ADDR="instantf-giveaway.herokuapp.com:443" -a instantf-xgateway
```
*Note: Because Heroku routes external traffic over HTTPS, ensure your gRPC client connections are configured to use TLS in production, rather than `insecure.NewCredentials()`.*
