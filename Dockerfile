# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o financial-tracker ./internal/cmd/api

# Run stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for HTTPS (outbound calls to ledger-service)
RUN apk --no-cache add ca-certificates

# Copy binary from builder
COPY --from=builder /app/financial-tracker .

# Expose port
EXPOSE 8081

# Run the application
CMD ["./financial-tracker"]
