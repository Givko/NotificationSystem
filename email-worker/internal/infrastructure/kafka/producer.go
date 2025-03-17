package kafka

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/Givko/NotificationSystem/email-worker/internal/utils"
	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"
)

// Producer is the interface for sending Kafka messages.
type Producer interface {
	Produce(ctx context.Context, topic string, key []byte, value []byte) error
	Close(ctx context.Context) error
}

var _ Producer = (*kafkaProducer)(nil)

// Config holds configuration options for the Kafka producer.
type Config struct {
	BootstrapServers     string // Comma-separated list of brokers: "broker1:9092,broker2:9092"
	RequiredAcks         int    // e.g. 1 or kafka.RequireAll (see kafka-go docs)
	MaxRetries           int    // Maximum number of retry attempts
	DeadLetterTopic      string // (optional)DLQ topic name
	MessageChannelBuffer int

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

	closeOnce   sync.Once
	closed      bool
	closedMutex sync.RWMutex
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
		msgChan:         make(chan kafka.Message, cfg.MessageChannelBuffer), // make configurable
		quit:            make(chan struct{}),
		closed:          false,
		closedMutex:     sync.RWMutex{},
	}

	kp.logger.Info().
		Str("bootstrap.servers", cfg.BootstrapServers).
		Msg("Kafka producer created successfully")

	kp.startWorker()

	return kp, nil
}

// Produce sends a message to a given topic. If sending fails, a retry is scheduled.
func (kp *kafkaProducer) Produce(ctx context.Context, topic string, key []byte, value []byte) error {
	kp.closedMutex.RLock()
	if kp.closed {
		kp.closedMutex.RUnlock()
		return errors.New("producer is closed")
	}

	kp.closedMutex.RUnlock()

	msg := kafka.Message{
		Topic:   topic,
		Key:     key,
		Value:   value,
		Headers: []kafka.Header{},
	}

	select {
	case kp.msgChan <- msg:
		kp.logger.Debug().Str("topic", topic).Msg("Enqueued message for asynchronous production")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close gracefully shuts down the producer.
func (kp *kafkaProducer) Close(ctx context.Context) error {
	var closeErr error
	kp.closeOnce.Do(func() {
		kp.logger.Info().Msg("Closing Kafka producer...")

		kp.closedMutex.Lock()
		kp.closed = true
		kp.closedMutex.Unlock()

		close(kp.msgChan)

		close(kp.quit)

		done := make(chan struct{})
		go func() {
			kp.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-ctx.Done():
			kp.logger.Warn().Msg("Timeout waiting for worker to finish")
			closeErr = ctx.Err()
		}

		if writerErr := kp.writer.Close(); writerErr != nil {
			kp.logger.Error().Err(writerErr).Msg("Kafka writer close failed")
			closeErr = errors.Join(closeErr, writerErr) // Go 1.20+
		}
	})

	return closeErr
}

// startWorker launches a background goroutine to process enqueued messages.
func (kp *kafkaProducer) startWorker() {
	kp.wg.Add(1)
	go func() {
		defer kp.wg.Done()
		for {
			select {
			case msg, ok := <-kp.msgChan:
				if !ok {
					return
				}
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
	if utils.IsNilEmptyOrWhitespace(&kp.deadLetterTopic) || kp.deadLetterTopic == failedMsg.Topic {
		kp.logger.Error().
			Str("topic", failedMsg.Topic).
			Msg("Dead letter topic is not set or is the same as the failed message topic")
		return errors.New("dead letter topic is not set or is the same as the failed message topic")
	}

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

	var err error
	for attempt := 1; attempt <= kp.maxRetries; attempt++ {
		err = kp.writer.WriteMessages(ctx, *dlqMsg)
		if err == nil {
			return nil
		}
		time.Sleep(time.Second * time.Duration(attempt)) // Simple backoff
	}

	kp.logger.Error().Msg("DLQ write failed after retries; message lost")
	return err
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
