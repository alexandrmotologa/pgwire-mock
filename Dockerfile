# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/pgwire-mock ./cmd/pgwire-mock

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /bin/pgwire-mock /usr/local/bin/pgwire-mock
COPY examples/ /app/examples/

EXPOSE 5432 8080

ENTRYPOINT ["pgwire-mock"]
CMD ["--port", "5432", "--admin-port", "8080", "--rules", "/app/examples/rules_ecommerce.yaml", "-v"]
