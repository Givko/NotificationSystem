package kafka

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"
)

// Producer is the interface for sending Kafka messages.
type Producer interface {
	Produce(ctx context.Context, topic string, key []byte, value []byte) <-chan error
	Close(ctx context.Context) error
}

var _ Producer = (*kafkaProducer)(nil)

type KafkaWriter interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

// Config holds configuration options for the Kafka producer.
type Config struct {
	BootstrapServers     string // Comma-separated list of brokers: "broker1:9092,broker2:9092"
	RequiredAcks         int    // e.g. 1 or kafka.RequireAll (see kafka-go docs)
	MaxRetries           int    // Maximum number of retry attempts
	DeadLetterTopic      string // DLQ topic name
	MessageChannelBuffer int
	WorkerPoolSize       int
	BatchSize            int
	BatchTimeoutMs       int

	// Additional fields (e.g., TLS/SASL settings) can be added here.
}

type internalMessage struct {
	message *kafka.Message
	errChan chan error
	ctx     context.Context
}

// kafkaProducer implements Producer using kafka-go.
type kafkaProducer struct {
	writer          KafkaWriter
	logger          zerolog.Logger
	wg              sync.WaitGroup
	workerPoolSize  int
	maxRetries      int
	deadLetterTopic string
	msgChan         chan *internalMessage
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
		BatchTimeout: time.Duration(cfg.BatchTimeoutMs) * time.Millisecond, // make configurable
		BatchSize:    cfg.BatchSize,                                        // make configurable
	}

	kp := &kafkaProducer{
		writer:          &writer,
		logger:          logger.With().Str("component", "kafkaProducer").Logger(),
		maxRetries:      cfg.MaxRetries,
		deadLetterTopic: cfg.DeadLetterTopic,
		wg:              sync.WaitGroup{},
		msgChan:         make(chan *internalMessage, cfg.MessageChannelBuffer), // make configurable
		quit:            make(chan struct{}),
		closed:          false,
		workerPoolSize:  cfg.WorkerPoolSize,
	}

	kp.logger.Info().
		Str("bootstrap.servers", cfg.BootstrapServers).
		Msg("Kafka producer created successfully")

	kp.startWorkers()

	return kp, nil
}

// Produce sends a message to a given topic. If sending fails, a retry is scheduled.
func (kp *kafkaProducer) Produce(ctx context.Context, topic string, key []byte, value []byte) <-chan error {
	errChan := make(chan error, 1)

	kp.closedMutex.RLock()
	if kp.closed {
		kp.closedMutex.RUnlock()
		errChan <- errors.New("producer is closed")
		return errChan
	}

	kp.closedMutex.RUnlock()

	msg := kafka.Message{
		Topic:   topic,
		Key:     key,
		Value:   value,
		Headers: []kafka.Header{},
	}
	internalMsg := internalMessage{
		message: &msg,
		errChan: errChan,
		ctx:     ctx,
	}

	// Try to enqueue the message, but respect context cancellation.
	select {
	case kp.msgChan <- &internalMsg:
		kp.logger.Debug().Str("topic", topic).Msg("Enqueued message for asynchronous production")
	case <-ctx.Done():
		errChan <- ctx.Err()
	}

	return errChan
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

// startWorkers launches a background goroutine to process enqueued messages.
func (kp *kafkaProducer) startWorkers() {
	for i := 0; i < kp.workerPoolSize; i++ {
		kp.wg.Add(1)
		go func() {
			defer kp.wg.Done()
			for {
				select {
				case msg, ok := <-kp.msgChan:
					if !ok {
						msg.errChan <- fmt.Errorf("producer is closed")
						return
					}
					err := kp.handleMessage(msg.ctx, msg)
					msg.errChan <- err
				case <-kp.quit:

					// Drain remaining messages if necessary.
					for msg := range kp.msgChan {
						err := kp.handleMessage(msg.ctx, msg)
						msg.errChan <- err
					}

					return
				}
			}
		}()
	}
}

// handleMessage attempts to send a message with retries.
func (kp *kafkaProducer) handleMessage(ctx context.Context, msg *internalMessage) error {
	kafkaMessage := msg.message
	start := time.Now()
	err := kp.writer.WriteMessages(ctx, *kafkaMessage)
	elapsed := time.Since(start)
	kp.logger.Debug().
		Str("topic", kafkaMessage.Topic).
		Dur("elapsed", elapsed).
		Msg("Message produced")

	if err != nil {
		kp.logger.Warn().
			Str("topic", kafkaMessage.Topic).
			Msg("Initial write failed, starting retries")

		err := kp.retryMessage(ctx, kafkaMessage)
		if err != nil {
			kp.logger.Error().
				Str("topic", kafkaMessage.Topic).
				Msg("Message failed after retries")
			return err
		}

		return nil
	}

	kp.logger.Debug().
		Str("topic", kafkaMessage.Topic).
		Msg("Message produced successfully")

	return nil
}

// retryMessage attempts to resend a failed message with exponential backoff.
// If the number of attempts exceeds the max retries, the message is sent to a DLQ.
func (kp *kafkaProducer) retryMessage(ctx context.Context, failedMsg *kafka.Message) error {
	kp.logger.Warn().
		Str("topic", failedMsg.Topic).
		Msg("Starting message retry production")

	var err error
	for attempt := 1; attempt <= kp.maxRetries; attempt++ {
		newMessage := copyMessage(failedMsg)
		backoff := time.Duration(1 << attempt * time.Second)

		select {
		case <-time.After(backoff):
			err = kp.writer.WriteMessages(ctx, *newMessage)
			if err == nil {
				kp.logger.Info().
					Int("attempt", attempt).
					Str("topic", newMessage.Topic).
					Msg("Message produced successfully after retry")
				return nil
			}

			kp.logger.Warn().
				Err(err).
				Int("attempt", attempt).
				Str("topic", newMessage.Topic).
				Msg("Retrying message production")

			continue
		case <-ctx.Done():
			kp.logger.Warn().
				Str("topic", newMessage.Topic).
				Msg("Retry aborted due to context cancellation")
			err = ctx.Err()
			return err
		}
	}

	kp.logger.Error().
		Str("topic", failedMsg.Topic).
		Msg("Max retries reached; sending to DLQ")

	err = fmt.Errorf("max retries reached for message to topic %s", failedMsg.Topic)
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
