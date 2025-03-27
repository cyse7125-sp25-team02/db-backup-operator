# Build stage for the manager binary
FROM --platform=$BUILDPLATFORM docker.io/golang:1.23 AS builder
ARG TARGETOS=linux
ARG TARGETARCH

WORKDIR /workspace

# Copy Go module files
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

# Copy source code
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/ internal/

# Build the manager binary for the target platform
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

# Final stage: Combine manager and backup tools
FROM gcr.io/google.com/cloudsdktool/cloud-sdk:slim

# Install PostgreSQL client tools (for pg_dump)
RUN apt-get update && apt-get install -y postgresql-client && rm -rf /var/lib/apt/lists/*

# Copy the manager binary from the builder stage
COPY --from=builder /workspace/manager /usr/local/bin/manager

# Set working directory
WORKDIR /app

# Default entrypoint is the manager, but can be overridden for the Job
ENTRYPOINT ["/usr/local/bin/manager"]
