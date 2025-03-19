# Production Deployment Guide

This guide outlines the architecture, deployment strategies, and implementation details for running the Notification System in a production environment.

## System Architecture

### Core Services
- **Notification Orchestrator**: API service for accepting notification requests
- **Channel Workers**: Specialized microservices (Email, SMS, Slack) processing notifications
- **Kafka Cluster**: Message broker with dedicated topics per channel
- **User Preferences Database**: Stores user opt-in/opt-out settings

### Deployment Diagram

```
                          ┌─────────────────┐
                          │   Load Balancer │
                          └────────┬────────┘
                                   │
                         ┌─────────▼─────────┐
                         │    Orchestrator   │
                         │    Service (N)    │
                         └─────────┬─────────┘
                                   │
                         ┌─────────▼─────────┐
                         │   Kafka Cluster   │
                         └───┬───────┬───────┘
                             │       │       │
               ┌─────────────┘       │       └──────────────┐
               │                     │                      │
      ┌────────▼───────┐    ┌────────▼───────┐     ┌────────▼───────┐
      │  Email Worker  │    │   SMS Worker   │     │  Slack Worker  │
      │  Service (N)   │    │   Service (N)  │     │   Service (N)  │
      └────────┬───────┘    └────────┬───────┘     └────────┬───────┘
               │                     │                      │
      ┌────────▼───────┐    ┌────────▼───────┐     ┌────────▼───────┐
      │   Email API    │    │    SMS API     │     │   Slack API    │
      └────────────────┘    └────────────────┘     └────────────────┘
```

## Container Orchestration

### Kubernetes Configuration

```yaml
# Example Kubernetes deployment for the orchestrator
apiVersion: apps/v1
kind: Deployment
metadata:
  name: notification-orchestrator
spec:
  replicas: 3
  selector:
    matchLabels:
      app: notification-orchestrator
  template:
    metadata:
      labels:
        app: notification-orchestrator
    spec:
      containers:
      - name: orchestrator
        image: jivkomilev/notification-orchestrator:latest
        ports:
        - containerPort: 8080
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
        resources:
          limits:
            cpu: "500m"
            memory: "512Mi"
          requests:
            cpu: "200m"
            memory: "256Mi"
```

## Messaging Infrastructure

### Kafka Configuration

- **Production Setup**: 3+ brokers with replication factor of 3
- **Topics**: Dedicated topics per notification channel
- **Retention**: 7-day message retention with log compaction
- **Serialization**: Protocol Buffers (Protobuf) or Avro for efficient encoding if needed
- **Compations**: We might use compaction if needed to store latest message in topic
- **Schema registry**: We can use schema registry to register and get schemas if necessary 

### Topic Configuration
Example
```yaml
topics:
  - name: email-worker
    partitions: 6
    replication: 3
    retention.ms: 604800000  # 7 days
  - name: sms-worker
    partitions: 6
    replication: 3
    retention.ms: 604800000  # 7 days
  - name: slack-worker
    partitions: 6
    replication: 3
    retention.ms: 604800000  # 7 days  
  - name: email-worker-dlq
    partitions: 6
    replication: 3
    retention.ms: 604800000  # 7 days
  - name: sms-worker-dlq
    partitions: 6
    replication: 3
    retention.ms: 604800000  # 7 days
  - name: slack-worker-dlq
    partitions: 6
    replication: 3
    retention.ms: 604800000  # 7 days
```

## User Preferences Database

### Schema

```sql
CREATE TABLE notification_preferences (
    user_id VARCHAR(255) NOT NULL,
    channel VARCHAR(50) NOT NULL,
    opt_in BOOLEAN NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, channel)
);

CREATE INDEX idx_notification_preferences_user_id ON notification_preferences(user_id);
```

### Integration Flow

1. Notification request received
2. Orchestrator checks user preferences database
3. If opted in, message sent to appropriate Kafka topic
4. If opted out, request is logged and skipped

## Provider Integration

### Current Status
Each worker service uses a consistent framework but lacks provider-specific integration:

### Implementation Plan

#### 1. Provider Interface
```go
type NotificationSender interface {
    Send(ctx context.Context, notification *model.Notification) error
}
```

#### 2. Provider Implementations
```go
// Email Provider
type EmailSender struct {
    smtpConfig *SMTPConfig
}

func (s *EmailSender) Send(ctx context.Context, notification *model.Notification) error {
    // Implement SMTP or Email API integration
}

// Similar implementations for SMS and Slack
```

## Observability

### Metrics Collection

- **Service Metrics**: Request rates, error rates, processing time
- **Kafka Metrics**: Consumer lag, producer throughput
- **Custom Business Metrics**: Notifications sent per channel, delivery success rate

### Alert Configuration
Example
```yaml
alerts:
  - name: HighErrorRate
    condition: sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m])) > 0.05
    severity: critical
    notification:
      - pagerduty
  - name: KafkaConsumerLag
    condition: sum(kafka_consumer_lag) by (topic) > 1000
    severity: warning
    notification:
      - slack
      - email
```

## Security Implementation

- **API Authentication**: JWT with role-based access control if needed
- **Kafka Security**: SASL/SCRAM authentication with TLS encryption if needed
- **Secret Management**: HashiCorp Vault for credentials and API keys

## Deployment Pipeline

```
┌─────────┐     ┌───────────┐     ┌─────────┐     ┌────────────┐     ┌───────────┐
│   Git   │ ──► │   Build   │ ──► │  Test   │ ──► │  Publish   │ ──► │  Deploy   │
│  Push   │     │  Service  │     │ Service │     │ Container  │     │    to     │
└─────────┘     └───────────┘     └─────────┘     └────────────┘     │ Staging/  │
                                                                     │ Production │
                                                                     └───────────┘
```

## Scalability Guidelines/Examples

### Horizontal Scaling Thresholds

| Service | CPU Threshold | Memory Threshold | Scaling Max |
|---------|---------------|------------------|-------------|
| Orchestrator | 70% | 75% | 10 replicas |
| Workers | 60% | 65% | 20 replicas per type |

### Vertical Scaling Recommendations

- Start with current resource allocations
- Monitor application performance metrics
- Adjust resource limits based on observed usage patterns
- Consider separate instance types for workers vs. orchestrator

## On-Call Operations

### Runbook Example: High Error Rate

1. **Check Logs**: Review error logs in centralized logging system
2. **Verify External Services**: Check status of provider APIs
3. **Review Recent Deployments**: Correlation with recent changes
4. **Remediation Steps**:
   - Roll back recent changes if applicable
   - Scale up resources if load-related
   - Implement circuit breaker if provider API issue

### Escalation Policy Example

| Time | Action |
|------|--------|
| 0 min | Primary on-call engineer notified |
| 15 min | Secondary on-call engineer notified |
| 30 min | Engineering manager notified |
| 45 min | CTO notified |