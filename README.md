# Notification System

This repository contains a **notification orchestration service** designed to reliably send notifications (email, SMS, Slack, etc.) via multiple channels. The system is designed for **horizontal scalability**, **at-least-once delivery**, and **ease of extension** to add new notification channels or features like rate limiting and authentication.

---

## Table of Contents
1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Key Components](#key-components)
4. [Workflow](#workflow)
5. [Existing Features](#existing-features)
6. [Planned or Extensible Features](#planned-or-extensible-features)
7. [Local Development and Deployment](#local-development-and-deployment)
8. [Configuration](#configuration)
9. [Observability and Monitoring](#observability-and-monitoring)
10. [Contributing](#contributing)
11. [License](#license)

---

## Overview

The **Notification System** provides a central API for multiple microservices (or external clients) to send notifications to end users. It decouples the notification sending logic from the business logic of individual services, making it easier to manage, monitor, and scale.

**Key goals:**
- **Scalability:** The system can scale horizontally to handle high traffic.
- **Reliability:** Notifications have an at-least-once delivery guarantee, with a built-in retry mechanism.
- **Extensibility:** Easily add new channels (e.g., Slack, SMS, Email, Push) or switch out third-party providers.
- **Maintainability:** Clear separation of concerns (API, service layer, producer/consumer model).

---

## Architecture

Below is a high-level conceptual diagram illustrating how the system is structured:

                +---------------------+
                |   Service 1 to N   |
                +---------+----------+
                          |
                          v
              +--------------------------+
              |    Notification Service  |
              | (Orchestrator/HTTP API)  |
              +-----------+--------------+
                          |  (Kafka)
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


### Key Architecture Points

1. **Notification Service (Orchestrator)**
   - Exposes an HTTP API for incoming notification requests.
   - Performs basic validation and passes messages to Kafka topics.
2. **Message Queues (Kafka)**
   - Decouples the notification service from the workers.
   - Provides buffering and retry capabilities.
3. **Workers**
   - Consume messages from respective Kafka topics (Slack, Email, SMS).
   - Send notifications to third-party services (e.g., Slack, email providers, SMS gateways).
4. **Database and Cache (Optional/Pluggable)**
   - Store user preferences, templates, or rate-limiting counters.
5. **Dead-Letter Queue (DLQ)**
   - Handles messages that fail after multiple retries.

---

## Key Components

1. **HTTP Server (Gin)**
   - Receives notification requests (`/api/v1/notifications`) and forwards them to the service layer.

2. **Service Layer**
   - Encapsulates business logic around notification creation, validation, and queuing.
   - Uses the **Kafka Producer** to enqueue messages.

3. **Kafka Producer**
   - Writes messages to the appropriate topic based on the notification channel.
   - Implements retries with exponential backoff and a dead-letter queue mechanism.

4. **Configuration Manager (Viper)**
   - Reads YAML configuration for server ports, Kafka settings, channel topics, etc.
   - Allows hot-reloading of config changes.

5. **Metrics & Logging**
   - **Prometheus** metrics for HTTP request durations, etc.
   - **zerolog** for structured logging in both development and production modes.

6. **Workers (Separate Services)**
   - Pull messages from channel-specific Kafka topics (e.g., `slack-worker`, `email-worker`, `sms-worker`).
   - Interact with third-party APIs to deliver notifications.

---

## Workflow

1. **Incoming Request**
   - A client (e.g., `Service 1`) sends a `POST /api/v1/notifications` request with JSON payload specifying channel, recipient, and message details.

2. **Validation & Processing**
   - The notification service validates the request (e.g., checks if email or phone number is well-formed).
   - (Optional) Fetches additional data from a **cache** or **database** (e.g., user preferences, templates).

3. **Enqueue Message**
   - A Kafka producer writes the notification event to the corresponding topic (e.g., `slack-worker`, `email-worker`, `sms-worker`).

4. **Workers Consume**
   - Separate worker services consume messages from Kafka, construct the final payload, and call third-party APIs (Slack, email providers, SMS gateways).

5. **Delivery**
   - Third-party providers deliver the notifications to end users.

6. **Retry & DLQ**
   - If a worker fails to deliver, it retries a configurable number of times.
   - If all retries fail, the message is sent to a **dead-letter queue (DLQ)**.

---

## Existing Features

- **HTTP API Endpoint** (`POST /api/v1/notifications`)
  - Accepts JSON payloads with fields like `recipient`, `recipient_id`, `sender`, `sender_id`, `subject`, `message`, and `channel`.
- **At-Least-Once Delivery**
  - Achieved via Kafka and retry logic. Failed messages go to a DLQ.
- **Horizontal Scalability**
  - The notification service (orchestrator) can be scaled horizontally behind a load balancer.
  - Kafka-based message queues decouple producers from consumers.
- **Metrics & Observability**
  - Prometheus endpoint (`/metrics`) for tracking HTTP request durations.
  - Structured logs with `zerolog`.
- **Graceful Shutdown**
  - The service listens for SIGINT/SIGTERM signals and shuts down HTTP and Kafka producer gracefully.

---

## Planned or Extensible Features

1. **Rate Limiting**
   - Prevents sending too many notifications to the same user.
   - Could be implemented with a **Redis** or in-memory token bucket approach.
2. **Authentication & Authorization**
   - Only verified internal services or authenticated clients can call the notification API.
   - Could be done via OAuth 2.0, JWT, or mutual TLS.
3. **Notification Templates**
   - Store and manage reusable message templates for consistent formatting.
   - Allows dynamic parameters and personalization.
4. **User Notification Settings**
   - Respect user preferences (opt-in/opt-out) for each channel.
   - Store in a database or cache for quick lookups.
5. **Monitoring and Analytics**
   - Track open rates, click rates, and bounce rates to measure notification effectiveness.
   - Alert on high DLQ volumes or third-party service outages.
6. **Workers**
   - While this repository focuses on the orchestrator, worker services handle actual sending.
   - Each worker can be scaled independently based on traffic per channel.

---

## Local Development and Deployment

1. **Clone the Repository**
   ```bash
   git clone https://github.com/YourOrg/notification-service.git
   cd notification-service

To be finished