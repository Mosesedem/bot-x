FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY . .

WORKDIR /app/services/kyc
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/service ./cmd/main.go

FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=builder /bin/service /app/service
# Heroku dynamically assigns the PORT, but we expose 8080 as a default documentation
EXPOSE 8080
USER nonroot:nonroot
CMD ["/app/service"]
