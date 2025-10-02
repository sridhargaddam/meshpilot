# Multi-stage build for MeshPilot MCP Server
FROM golang:1.23-alpine AS builder

# Install build dependencies and kubectl
RUN apk add --no-cache git ca-certificates tzdata curl && \
    curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl" && \
    chmod +x kubectl && \
    mv kubectl /usr/local/bin/

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o meshpilot main.go

# Final stage - minimal image
FROM scratch

# Copy certificates from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy the binary and kubectl
COPY --from=builder /app/meshpilot /meshpilot
COPY --from=builder /usr/local/bin/kubectl /usr/local/bin/kubectl

# Set environment variables
ENV TZ=UTC

# Expose MCP server port (if applicable)
EXPOSE 8080

# Add labels for better container management
LABEL maintainer="meshpilot-team" \
      description="MeshPilot MCP Server for Kubernetes Istio Management" \
      version="0.1.0"

# Run the binary
ENTRYPOINT ["/meshpilot"]
