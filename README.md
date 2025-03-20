# Notification System

A scalable notification service that supports email, SMS, and Slack channels with at-least-once delivery guarantee.

## Overview

This system provides a centralized service for sending notifications across multiple channels with horizontal scalability and reliability. Built with Go, it uses Kafka as a message broker to ensure reliable delivery.

## Architecture

- **Notification Orchestrator**: API service that receives and routes notification requests
- **Channel Workers**: Dedicated services for email, SMS, and Slack delivery
- **Kafka**: Message broker ensuring reliable message delivery
- **Prometheus**: Metrics and monitoring

## Quick Start

### Prerequisites

- Docker and Docker Compose

### Running the System

1. Clone the repository:
```bash
git clone https://github.com/Givko/NotificationSystem
cd NotificationSystem
```

2. Start all services:
```bash
docker-compose up --wait
```

OR build all services from source:
```bash
docker-compose up --build --wait
```

OR run only dependencies for local development:
```bash
docker-compose up zookeeper kafka kafka-init
```

3. Send a test notification:
```bash
curl -X POST http://localhost:8081/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{
    "recipient": "John Doe",
    "recipient_id": "user123",
    "sender": "Service XYZ",
    "sender_id": "service_xyz",
    "subject": "Important Update",
    "message": "Your order has been shipped!",
    "channel": "email"
  }'
```

## Service Components

| Service | Port | Description |
|---------|------|-------------|
| notification-orchestrator | 8081 | Main API service |
| email-worker | 8080 | Processes email notifications |
| sms-worker | 8082 | Processes SMS notifications |
| slack-worker | 8083 | Processes Slack notifications |
| kafka | 9092 | Message broker |
| prometheus | 9090 | Metrics and monitoring |

## Configuration

The system uses environment variables for configuration. Key settings:

```yaml
server:
  port: 8080
kafka:
  bootstrap-servers: localhost:9092 # for using docker use kafka:29092
notifications:
  channel-topics:
    slack: slack-worker
    email: email-worker
    sms: sms-worker
```

## Development

### Running Tests

```bash
go test ./...
```

### Project Structure

```
project-root/
├── go.mod                             # Single go.mod file for the entire project
├── go.sum                             # Dependencies lockfile
├── cmd/                               # Contains main packages for each executable
│   ├── email-worker/
│   │   └── main.go                    # Main entry point for email-worker service
│   └── notification-orchestrator/
│       └── main.go                    # Main entry point for notification-orchestrator service
├── internal/                          # Private application code
│   ├── email-worker/                  # Email-specific code
│   └── notification/                  # Notification-specific code
├── pkg/                               # Public libraries that can be imported by other projects
│   └── shared/                        # Shared code between services
└── docker/                            # Dockerfiles for each service
    ├── email-worker/
    │   └── Dockerfile
    └── notification-orchestrator/
        └── Dockerfile
```

## CI/CD

This project uses GitHub Actions for continuous integration and delivery. The workflow includes:

- **Linting**: Code quality checks using golangci-lint
- **Testing**: Automated tests with coverage reports
- **Building**: Compilation verification for all services
- **Docker Images**: Building and pushing Docker images to Docker Hub

### Docker Hub Repositories

Official Docker images are available at:
- [notification-orchestrator](https://hub.docker.com/repository/docker/jivkomilev/notification-orchestrator/general)
- [email-worker](https://hub.docker.com/repository/docker/jivkomilev/email-worker/general)
- [sms-worker](https://hub.docker.com/repository/docker/jivkomilev/sms-worker/general)
- [slack-worker](https://hub.docker.com/repository/docker/jivkomilev/slack-worker/general)

Pull images directly:
```bash
docker pull jivkomilev/notification-orchestrator:latest
docker pull jivkomilev/email-worker:latest
docker pull jivkomilev/sms-worker:latest
docker pull jivkomilev/slack-worker:latest
```

## Monitoring

Access Prometheus metrics at: http://localhost:9090
