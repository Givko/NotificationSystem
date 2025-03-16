# Notification System

A scalable, reliable notification service built with Go that supports multiple notification channels and guarantees at-least-once delivery.

## Overview

This notification system provides a centralized service for sending notifications via multiple channels (email, SMS, Slack) with easy extensibility for future channels. The system is designed with horizontal scalability, reliability, and observability in mind.

![Notification System Architecture](https://example.com/notification-system-architecture.png)

## Key Features

- **Multi-channel Support**: Send notifications via email, SMS, and Slack
- **Horizontal Scalability**: Stateless design allows for easy scaling
- **At-least-once Delivery**: Guaranteed delivery through retry mechanisms and dead letter queues
- **Extensibility**: Easily add new notification channels
- **Observability**: Comprehensive metrics and logging for system monitoring
- **Graceful Shutdown**: Proper handling of system signals for clean termination

## System Architecture

                        +--------------------+
                        |   Service 1 to N   |
                        +---------+----------+
                                 |
                                 v
                     +--------------------------+
                     |    Notification Service  |
                     | (Orchestrator/HTTP API)  |
                     +-----------+--------------+
                                 |
                                 v  (Kafka)
                  +---------------+----------------+
                  |               |                |
                  v               v                v
            +-------------+   +------------+   +------------+
            |  Slack Q    |   |  Email Q   |   |   SMS Q    |
            +------+------+   +-----+------+   +-----+------+
                  |                |                |
                  v                v                v
            +-------------+   +------------+   +------------+
            | SlackWorker |   |EmailWorker |   | SMSWorker  |
            +------+------+   +-----+------+   +-----+------+
                  |                |                |
                  v                v                v
            +-------------------------------------------------+
            |   Third-Party Services (Slack, Email, SMS, etc.)|
            +-------------------------------------------------+

### Components

#### Notification Service API

The notification service exposes HTTP endpoints for clients to submit notification requests. It:
- Validates incoming requests
- Routes notifications to appropriate Kafka topics based on channel type
- Provides metrics for monitoring request handling

#### Message Queue (Kafka)

Kafka serves as the backbone of our notification system:
- Decouples producers from consumers
- Enables horizontal scaling of workers
- Provides persistence of notification messages
- Supports retry mechanisms through topic configuration
- Maintains order of notifications when needed

#### Workers

Worker services consume messages from Kafka topics and deliver them to external services:
- Each channel (email, SMS, Slack) has dedicated workers
- Workers can be scaled independently based on channel-specific load
- Implement channel-specific retry logic and error handling

#### Dead Letter Queue

Failed notifications (after retries) are sent to a dedicated DLQ topic:
- Preserves messages that couldn't be delivered
- Allows for manual inspection and intervention
- Provides metrics on delivery failures

### Flows

#### Notification Submission Flow

1. Client submits notification via HTTP API
2. Service validates request and identifies target channel
3. Notification is serialized and published to channel-specific Kafka topic
4. Successful submission returns 202 Accepted status

#### Notification Processing Flow

1. Channel-specific worker consumes notification from Kafka
2. Worker attempts to deliver notification to external service
3. On success, the message is acknowledged
4. On failure, retry logic is applied
5. After max retries, notification is sent to DLQ

## Implementation Details

### Core Service

The notification service is implemented using the Gin framework and follows clean architecture principles:

- **Handlers**: Handle HTTP requests and responses
- **Services**: Contain business logic for notification processing
- **Infrastructure**: Provides implementation details for external systems (Kafka)

### Reliability Features

#### Retry Mechanism

The system implements a sophisticated retry mechanism:
- Exponential backoff for failed deliveries
- Configurable maximum retry attempts
- Dead letter queue for notifications that exceed retry limits

#### Graceful Shutdown

The service handles termination signals properly:
- Completes in-flight requests
- Flushes pending Kafka messages
- Releases resources in a controlled manner

### Observability

#### Logging

Structured logging using zerolog:
- JSON format for machine parsing in production
- Human-readable format for development
- Configurable log levels and output destinations

#### Metrics

Prometheus metrics for system monitoring:
- HTTP request duration
- Success/failure rates
- Queue depths
- Processing latencies

## Configuration

The system uses YAML configuration with environment variable overrides:

```yaml
server:
  port: 8080
kafka:
  bootstrap-servers: localhost:9092
  required-acks: 1
  max-retries: 3
notifications:
  channel-topics:
    slack: slack-worker
    email: email-worker
    sms: sms-worker
  dead-letter-topic: notifications-dlq
```

## Future Enhancements

### User Notification Preferences Database

A database to store user notification preferences would be implemented:
- Store opt-in/opt-out preferences per user and channel
- Enable notification template customization
- Track notification history and analytics

Implementation approach:
```go
type NotificationPreference struct {
    UserID      string
    Channel     string
    OptIn       bool
    UpdatedAt   time.Time
}

// Check user preferences before sending
func (s *notificationService) SendNotification(notification contracts.Notification) error {
    // Check if user has opted in for this channel
    if !s.preferencesRepository.HasUserOptedIn(notification.RecipientID, notification.Channel) {
        return ErrUserOptedOut
    }
    // Proceed with sending...
}
```

### Rate Limiting

To prevent notification fatigue and protect resources:
- Per-user rate limits across channels
- Global rate limits per channel type
- Custom rate limit rules for different notification types

Implementation approach using Redis:
```go
func (s *notificationService) SendNotification(notification contracts.Notification) error {
    // Check rate limits before sending
    exceeded, err := s.rateLimiter.CheckAndIncrement(notification.RecipientID, notification.Channel)
    if err != nil {
        return err
    }
    if exceeded {
        return ErrRateLimitExceeded
    }
    // Proceed with sending...
}
```

### Notification Templates

A template system to standardize notification formats:
- Store templates in a database or file system
- Support for variables and personalization
- Versioning of templates for tracking changes

### Analytics and Event Tracking

To measure effectiveness and engagement:
- Track delivery, open, and interaction rates
- A/B testing of notification content
- User engagement analysis

### Authentication and Authorization

Enhanced security features:
- API key authentication for service clients
- Role-based access control for notification management
- Audit logging for security compliance

## Getting Started

### Prerequisites

- Go 1.22 or higher
- Kafka cluster
- Docker and Docker Compose (for local development)

### Running Locally

1. Clone the repository
```bash
git clone https://github.com/Givko/NotificationSystem
cd NotificationSystem
```

2. Start Kafka with Docker Compose
```bash
docker-compose up -d
```

3. Run the service
```bash
go run cmd/notification-service/main.go
```

### Testing

Execute the test suite:
```bash
go test ./...
```

Send a test notification:
```bash
curl -X POST http://localhost:8080/api/v1/notifications \
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

## Deployment

The service is designed to be deployed in containerized environments:

### Kubernetes Deployment

Example Kubernetes manifests are provided for:
- Deployment with horizontal pod autoscaling
- ConfigMaps for configuration
- Service for network exposure
- Monitoring integration with Prometheus

### Scaling Considerations

- API servers can be scaled horizontally
- Kafka partitioning enables parallel processing
- Worker deployments can be scaled independently per channel
- Consider regional deployments for global services

## Contributing

1. Fork the repository
2. Create a feature branch
3. Commit your changes
4. Push to the branch
5. Create a new Pull Request
