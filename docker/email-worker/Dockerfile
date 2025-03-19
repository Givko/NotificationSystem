FROM golang:1.24.1-bullseye AS builder

WORKDIR /usr/src/app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Install dependencies
RUN go mod download && go mod verify

# Copy the entire project
COPY . .

# Make sure all required dependencies are installed
RUN go mod tidy

# Build the email-worker service
RUN CGO_ENABLED=0 GOOS=linux go build -o /usr/local/bin/app ./cmd/email-worker

# --- Final stage: Distroless
FROM gcr.io/distroless/static-debian11

LABEL maintainer="milevjivko@gmail.com"

COPY --from=builder /usr/local/bin/app /app/app

# If you have a config file, copy it (adjust path as needed)
COPY --from=builder /usr/src/app/internal/email-worker/config/config.yaml /app/config/config.yaml

WORKDIR /app
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/app"]