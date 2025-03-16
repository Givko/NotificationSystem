package kafka

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"
)

// Producer is the interface for sending Kafka messages.
type Producer interface {
	Produce(ctx context.Context, topic string, key []byte, value []byte) error
	Close()
}

var _ Producer = (*kafkaProducer)(nil)

// Config holds configuration options for the Kafka producer.
type Config struct {
	BootstrapServers string // Comma-separated list of brokers: "broker1:9092,broker2:9092"
	RequiredAcks     int    // e.g. 1 or kafka.RequireAll (see kafka-go docs)
	MaxRetries       int    // Maximum number of retry attempts
	DeadLetterTopic  string // DLQ topic name
	// Additional fields (e.g., TLS/SASL settings) can be added here.
}

// kafkaProducer implements Producer using kafka-go.
type kafkaProducer struct {
	writer          *kafka.Writer
	logger          zerolog.Logger
	wg              sync.WaitGroup
	maxRetries      int
	deadLetterTopic string
	msgChan         chan kafka.Message
	quit            chan struct{}
	closeOnce       sync.Once
}

// NewKafkaProducer creates and configures a new Kafka producer.
func NewKafkaProducer(logger zerolog.Logger, cfg Config) (Producer, error) {
	writer := kafka.Writer{
		Addr:         kafka.TCP(cfg.BootstrapServers),
		RequiredAcks: kafka.RequiredAcks(cfg.RequiredAcks),
	}

	kp := &kafkaProducer{
		writer:          &writer,
		logger:          logger.With().Str("component", "kafkaProducer").Logger(),
		maxRetries:      cfg.MaxRetries,
		deadLetterTopic: cfg.DeadLetterTopic,
		wg:              sync.WaitGroup{},
		msgChan:         make(chan kafka.Message, 1000), // make configurable
		quit:            make(chan struct{}),
	}

	kp.logger.Info().
		Str("bootstrap.servers", cfg.BootstrapServers).
		Msg("Kafka producer created successfully")

	kp.startWorker()

	return kp, nil
}

// Produce sends a message to a given topic. If sending fails, a retry is scheduled.
func (kp *kafkaProducer) Produce(ctx context.Context, topic string, key []byte, value []byte) error {
	msg := kafka.Message{
		Topic:   topic,
		Key:     key,
		Value:   value,
		Headers: []kafka.Header{}, // Start with no headers.
	}

	// Try to enqueue the message, but respect context cancellation.
	select {
	case kp.msgChan <- msg:
		kp.logger.Debug().Str("topic", topic).Msg("Enqueued message for asynchronous production")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close gracefully shuts down the producer.
func (kp *kafkaProducer) Close() {
	kp.closeOnce.Do(func() {
		kp.logger.Info().Msg("Closing Kafka producer...")
		close(kp.quit) // signal the worker to quit
		kp.wg.Wait()   // wait for worker to finish processing
		if err := kp.writer.Close(); err != nil {
			kp.logger.Error().Err(err).Msg("Error closing Kafka writer")
		}
	})
}

// startWorker launches a background goroutine to process enqueued messages.
func (kp *kafkaProducer) startWorker() {
	kp.wg.Add(1)
	go func() {
		defer kp.wg.Done()
		for {
			select {
			case msg := <-kp.msgChan:
				kp.handleMessage(context.Background(), msg)
			case <-kp.quit:

				// Drain remaining messages if necessary.
				for msg := range kp.msgChan {
					kp.handleMessage(context.Background(), msg)
				}

				return
			}
		}
	}()
}

// handleMessage attempts to send a message with retries.
func (kp *kafkaProducer) handleMessage(ctx context.Context, msg kafka.Message) {
	err := kp.writer.WriteMessages(ctx, msg)
	if err != nil {
		kp.logger.Warn().
			Str("topic", msg.Topic).
			Msg("Initial write failed, starting retries")
		success := kp.retryMessage(ctx, &msg)
		if !success {
			kp.logger.Error().
				Str("topic", msg.Topic).
				Msg("Message failed after retries, sent to DLQ")
		}
	} else {
		kp.logger.Debug().
			Str("topic", msg.Topic).
			Msg("Message produced successfully")
	}
}

// retryMessage attempts to resend a failed message with exponential backoff.
// If the number of attempts exceeds the max retries, the message is sent to a DLQ.
func (kp *kafkaProducer) retryMessage(ctx context.Context, failedMsg *kafka.Message) bool {
	kp.logger.Warn().
		Str("topic", failedMsg.Topic).
		Msg("Starting message retry production")

outerLoop:
	for attempt := 1; attempt <= kp.maxRetries; attempt++ {
		newMessage := copyMessage(failedMsg)
		backoff := time.Duration(1 << attempt * time.Second)

		select {
		case <-time.After(backoff):
			err := kp.writer.WriteMessages(ctx, *newMessage)
			if err != nil {
				kp.logger.Warn().
					Err(err).
					Int("attempt", attempt).
					Str("topic", newMessage.Topic).
					Msg("Retrying message production")

				continue
			} else {
				kp.logger.Info().
					Int("attempt", attempt).
					Str("topic", newMessage.Topic).
					Msg("Message produced successfully after retry")
				return true
			}
		case <-ctx.Done():
			kp.logger.Warn().
				Str("topic", newMessage.Topic).
				Msg("Retry aborted due to context cancellation")
			break outerLoop
		}
	}

	kp.logger.Error().
		Str("topic", failedMsg.Topic).
		Msg("Max retries reached; sending to DLQ")
	_ = kp.sendToDeadLetterQueue(ctx, failedMsg)

	return false
}

// sendToDeadLetterQueue sends the message to the configured DLQ, attaching additional metadata.
func (kp *kafkaProducer) sendToDeadLetterQueue(ctx context.Context, failedMsg *kafka.Message) error {
	dlqMsg := copyMessage(failedMsg)

	// Preserve the original topic for reference.
	originalTopic := dlqMsg.Topic
	dlqMsg.Topic = kp.deadLetterTopic
	dlqMsg.Headers = append(dlqMsg.Headers, kafka.Header{
		Key:   "original_topic",
		Value: []byte(originalTopic),
	})

	// Add a timestamp header for tracking.
	dlqMsg.Headers = append(dlqMsg.Headers, kafka.Header{
		Key:   "dlq_timestamp",
		Value: []byte(time.Now().Format(time.RFC3339)),
	})

	err := kp.writer.WriteMessages(ctx, *dlqMsg)
	if err != nil {
		kp.logger.Error().Err(err).Msg("Failed to deliver message to DLQ")

		// Optionally, add a fallback here (e.g., increment an error metric or trigger an alert).
		return err
	}

	kp.logger.Info().Msg("Message delivered to DLQ successfully")
	return nil
}

// copyMessage makes a deep copy of a kafka.Message.
func copyMessage(orig *kafka.Message) *kafka.Message {
	newHeaders := make([]kafka.Header, len(orig.Headers))
	copy(newHeaders, orig.Headers)
	newKey := slices.Clone(orig.Key)
	newValue := slices.Clone(orig.Value)

	return &kafka.Message{
		Topic:   orig.Topic,
		Key:     newKey,
		Value:   newValue,
		Headers: newHeaders,
	}
}
