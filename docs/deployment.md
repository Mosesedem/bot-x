# Deployment Guide: Heroku Container Registry

The InstantF Bot-X microservices architecture has been updated with production-ready, minimal-attack-surface Dockerfiles (`gcr.io/distroless/static`) and `heroku.yml` manifests for each service. 

Follow this guide to deploy the 10 microservices to Heroku using the Container Registry.

## Prerequisites
1. [Heroku CLI](https://devcenter.heroku.com/articles/heroku-cli) installed and authenticated (`heroku login`).
2. Docker installed and running locally.
3. A managed PostgreSQL database (e.g., Heroku Postgres, CockroachDB, Supabase).
4. A managed Redis instance (e.g., Heroku Data for Redis, Upstash).
5. A managed HashiCorp Vault instance (or HCP Vault Secrets) for production secrets management.

## 1. Create Heroku Apps

You need to create a separate Heroku app for each microservice. This ensures isolated scaling and crash boundaries.

```bash
# Example: Create apps with a unique prefix
PREFIX="instantf"
heroku create $PREFIX-xgateway
heroku create $PREFIX-giveaway
heroku create $PREFIX-entry
heroku create $PREFIX-payment-router
heroku create $PREFIX-kyc
heroku create $PREFIX-compliance
heroku create $PREFIX-audit
heroku create $PREFIX-notification
heroku create $PREFIX-reconciliation
heroku create $PREFIX-admin
```

## 2. Configure Environment Variables

For each app, you must explicitly set the necessary environment variables via the Heroku Dashboard or CLI. 
**Crucially, you must configure the gRPC addresses so the services can find each other over Heroku Private Networking/DNS.**

```bash
# Example for setting shared variables across all apps
for APP in xgateway giveaway entry payment-router kyc compliance audit notification reconciliation admin; do
  heroku config:set \
    APP_ENV=production \
    DATABASE_URL="postgres://user:pass@host:5432/db?sslmode=verify-full" \
    REDIS_URL="rediss://user:pass@host:6379" \
    VAULT_ADDR="https://your-vault-instance.com" \
    VAULT_TOKEN="your-prod-vault-token" \
    -a $PREFIX-$APP
done
```

### gRPC Service Discovery
Heroku exposes apps via HTTPS on port 443. Set the internal gRPC addresses for service-to-service communication:

```bash
# Set on all apps that need to communicate with others
for APP in xgateway giveaway entry payment-router kyc compliance audit notification reconciliation admin; do
  heroku config:set \
    GRPC_GIVEAWAY_ADDR="$PREFIX-giveaway.herokuapp.com:443" \
    GRPC_PAYMENT_ROUTER_ADDR="$PREFIX-payment-router.herokuapp.com:443" \
    GRPC_KYC_ADDR="$PREFIX-kyc.herokuapp.com:443" \
    GRPC_COMPLIANCE_ADDR="$PREFIX-compliance.herokuapp.com:443" \
    GRPC_AUDIT_ADDR="$PREFIX-audit.herokuapp.com:443" \
    GRPC_NOTIFICATION_ADDR="$PREFIX-notification.herokuapp.com:443" \
    GRPC_RECONCILIATION_ADDR="$PREFIX-reconciliation.herokuapp.com:443" \
    GRPC_ENTRY_ADDR="$PREFIX-entry.herokuapp.com:443" \
    GRPC_XGATEWAY_ADDR="$PREFIX-xgateway.herokuapp.com:443" \
    -a $PREFIX-$APP
done
```

## 3. Deployment

We use Heroku's Container Registry to push our multi-stage distroless Docker images.

1. **Log in to Container Registry:**
   ```bash
   heroku container:login
   ```

2. **Build and Push Each Service:**
   Navigate to the root of the project and execute the build/push commands for each service, passing the specific Dockerfile path.

   ```bash
   # XGateway
   heroku container:push web -a $PREFIX-xgateway --dockerfile services/xgateway/Dockerfile
   heroku container:release web -a $PREFIX-xgateway

   # Giveaway
   heroku container:push web -a $PREFIX-giveaway --dockerfile services/giveaway/Dockerfile
   heroku container:release web -a $PREFIX-giveaway
   
   # Repeat for entry, payment-router, kyc, compliance, audit, notification, reconciliation, admin
   ```

## 4. Run Database Migrations
Migrations should be run once against the production database before scaling up the dynos.

```bash
# Assuming you have golang-migrate installed locally
migrate -path migrations -database "postgres://user:pass@host:5432/db?sslmode=verify-full" up
```

## 5. Security Notes
- **Distroless Images:** The images use `gcr.io/distroless/static:nonroot`. They do not contain a shell (`/bin/sh` or `/bin/bash`). You cannot use `heroku run bash`. This significantly hardens the containers against remote code execution.
- **Fail-Closed Private Keys:** The `payment-router`, `reconciliation`, and `kyc` services will intentionally crash on boot if they cannot read the SafeHaven private key from Vault or the file system in production. Ensure Vault is correctly seeded before deploying these services.
