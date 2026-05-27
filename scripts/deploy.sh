#!/bin/bash
# ============================================================
#  bot-x — Deployment Script for Digital Ocean Droplet
#  Usage: ./scripts/deploy.sh [production|staging]
# ============================================================

set -e

ENVIRONMENT=${1:-production}
COMPOSE_FILE="docker-compose.prod.yml"

echo "=== Bot-X Deployment Script ==="
echo "Environment: $ENVIRONMENT"
echo ""

# Check prerequisites
command -v docker >/dev/null 2>&1 || { echo "Docker is required but not installed. Aborting." >&2; exit 1; }
command -v docker-compose >/dev/null 2>&1 || { echo "Docker Compose is required but not installed. Aborting." >&2; exit 1; }

# Check environment file
if [ ! -f .env ]; then
    echo "ERROR: .env file not found!"
    echo "Create one from .env.example and fill in production secrets."
    exit 1
fi

# Validate critical environment variables
echo "=== Validating Environment Variables ==="
required_vars=(
    "POSTGRES_PASSWORD"
    "ADMIN_JWT_SECRET"
    "X_BEARER_TOKEN"
    "X_WEBHOOK_SECRET"
)

for var in "${required_vars[@]}"; do
    if ! grep -q "^${var}=" .env || grep -q "^${var}=.*changeme\|^${var}=\$" .env; then
        echo "WARNING: $var is not set or contains placeholder value"
    fi
done

# Pull latest code if in a git repo
if [ -d .git ]; then
    echo "=== Pulling Latest Code ==="
    git pull origin main || echo "Warning: Could not pull latest code"
fi

# Build and deploy
echo "=== Building and Starting Services ==="
docker-compose -f $COMPOSE_FILE pull
docker-compose -f $COMPOSE_FILE build --no-cache
docker-compose -f $COMPOSE_FILE up -d

# Wait for services to be healthy
echo "=== Waiting for Services to Start ==="
sleep 10

# Check service status
echo "=== Service Status ==="
docker-compose -f $COMPOSE_FILE ps

# Run migrations explicitly (idempotent)
echo "=== Running Database Migrations ==="
docker-compose -f $COMPOSE_FILE run --rm migrate || echo "Migration may have already been applied"

# Clean up old images
echo "=== Cleaning Up Old Images ==="
docker image prune -f

# Health check
echo "=== Health Check ==="
if curl -s http://localhost:8080/health >/dev/null 2>&1; then
    echo "✅ xgateway is healthy"
else
    echo "⚠️  xgateway health check failed - check logs with: docker-compose -f $COMPILE_FILE logs xgateway"
fi

echo ""
echo "=== Deployment Complete ==="
echo "View logs: docker-compose -f $COMPOSE_FILE logs -f"
echo ""
