FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY . .

WORKDIR /app/services/audit
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/service ./cmd/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /bin/service /app/service
EXPOSE 8080 50051
CMD ["/app/service"]
