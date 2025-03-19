# Build stage
FROM golang:1.24.1-bullseye AS builder

WORKDIR /usr/src/app

# Copy Go module files first to leverage Docker layer caching
COPY go.mod go.sum ./
RUN go mod download && go mod verify && go mod tidy

# Copy your local shared library from ../shared into /usr/src/shared
# (Assumes your build context includes ../shared)
COPY ../shared /usr/src/shared

# Now copy all remaining application source code
COPY . .

# Build the binary (statically linked)
RUN CGO_ENABLED=0 GOOS=linux go build -o /usr/local/bin/app .

# Final stage: distroless image with non-root user
FROM gcr.io/distroless/static-debian11

LABEL maintainer="milevjivko@gmail.com"

# Copy the built binary from the builder stage
COPY --from=builder /usr/local/bin/app /app/app

# Copy the configuration file (as in your original example)
COPY --from=builder /usr/src/app/internal/config/config.yaml /app/internal/config/config.yaml

# Set working directory to /app so logs.log is created there
WORKDIR /app

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/app/app"]
