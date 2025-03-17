package kafka

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/Givko/NotificationSystem/email-worker/internal/infrastructure/metrics"
	"github.com/Givko/NotificationSystem/email-worker/internal/utils"
	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"
)

// Consumer defines the interface for consuming Kafka messages.
type Consumer interface {
	// Start begins the consumption loop. It reads messages from the configured topics,
	// applies the provided handler (with built-in retry logic), and commits offsets (if using manual commits).
	// This method blocks until the context is cancelled or an unrecoverable error occurs.
	Start(ctx context.Context, handler func(ctx context.Context, msg *kafka.Message) error) error

	// Close gracefully shuts down the consumer.
	Close() error
}

// ConsumerConfig holds configuration options for the Kafka consumer.
type ConsumerConfig struct {
	BootstrapServers      string        // Comma-separated list of brokers: "broker1:9092,broker2:9092"
	GroupID               string        // Consumer group identifier
	Topic                 string        // topic to subscribe to
	CommitInterval        time.Duration // If > 0, automatic commits occur at this interval; if 0, manual commits are required.
	MaxProcessingRetries  int           // Number of attempts to process a message before giving up
	DeadLetterTopic       string        // Topic name for the Dead Letter Queue (DLQ)
	ConsumerChannelBuffer int           // Size of the message channel buffer
}

// kafkaConsumer implements the Consumer interface using kafka-go.
type kafkaConsumer struct {
	reader *kafka.Reader
	logger zerolog.Logger
	config ConsumerConfig

	wg                 sync.WaitGroup
	msgChannel         chan kafka.Message
	closed             bool
	closedMutex        sync.Mutex
	metrics            metrics.Metrics
	deadLetterProducer Producer // Optional: if non-nil and DeadLetterTopic in the config is set, failed messages will be sent here
	closeOnce          sync.Once
}

// NewKafkaConsumer creates and configures a new Kafka consumer.
// It splits the comma-separated BootstrapServers into a slice of broker addresses.
func NewKafkaConsumer(logger zerolog.Logger, cfg ConsumerConfig, producer Producer, metrics metrics.Metrics) (Consumer, error) {
	if len(cfg.Topic) == 0 {
		return nil, errors.New("no topics provided")
	}

	// Split the bootstrap servers into a slice.
	brokers := strings.Split(cfg.BootstrapServers, ",")

	readerConfig := kafka.ReaderConfig{
		Brokers:        brokers,
		GroupID:        cfg.GroupID,
		Topic:          cfg.Topic,
		CommitInterval: cfg.CommitInterval, // When set to 0, manual commits are expected.
	}

	reader := kafka.NewReader(readerConfig)
	consumer := &kafkaConsumer{
		reader:             reader,
		logger:             logger.With().Str("component", "kafkaConsumer").Logger(),
		config:             cfg,
		closed:             false,
		deadLetterProducer: producer,
		metrics:            metrics,
		wg:                 sync.WaitGroup{},
		msgChannel:         make(chan kafka.Message, cfg.ConsumerChannelBuffer), // make configurable
	}

	logger.Info().
		Str("topic", cfg.Topic).
		Str("group", cfg.GroupID).
		Msg("Kafka consumer created successfully")

	return consumer, nil
}

// Start begins the message consumption loop. It reads messages from Kafka,
// processes them using the provided handler with built-in retry logic,
// and commits offsets if manual commits are in use.
func (kc *kafkaConsumer) Start(ctx context.Context, handler func(ctx context.Context, msg *kafka.Message) error) error {
	defer kc.Close()

	kc.wg.Add(1)
	go func() {
		defer kc.wg.Done()
		kc.consumeMessages(ctx, handler)
	}()

	for {
		if ctx.Err() != nil {
			kc.logger.Info().Msg("Context canceled, stopping consumer")
			return nil
		}

		// Read the next message (this call will block)
		msg, err := kc.reader.ReadMessage(ctx)
		if err != nil {
			// When the context is cancelled or the reader is closed, exit gracefully.
			if errors.Is(err, context.Canceled) {
				kc.logger.Info().Msg("Context canceled, stopping consumer")
				return nil
			}

			kc.logger.Error().Err(err).Msg("Error reading message")
			continue
		}

		select {
		case kc.msgChannel <- msg:
		case <-ctx.Done():
			kc.logger.Info().Msg("Context canceled, stopping consumer")
			return nil
		}
	}
}

func (kc *kafkaConsumer) consumeMessages(ctx context.Context, handler func(ctx context.Context, msg *kafka.Message) error) {
	for {
		select {
		case msg, ok := <-kc.msgChannel:
			if !ok {
				kc.logger.Info().Msg("Message channel closed, stopping message processing")
				return
			}

			kc.wg.Add(1)
			go func(msg kafka.Message) {
				defer kc.wg.Done()
				kc.processMessage(ctx, msg, handler)
			}(msg)
		case <-ctx.Done():
			kc.logger.Info().Msg("Context canceled, stopping message processing")
			return
		}
	}
}

func (kc *kafkaConsumer) processMessage(ctx context.Context, msg kafka.Message, handler func(ctx context.Context, msg *kafka.Message) error) {
	start := time.Now()
	// Process the message with retry logic.
	success := false
	defer func() {
		kc.metrics.ObserveHTTPRequestDuration(msg.Topic, success, time.Since(start).Seconds())
	}()

	for attempt := 0; attempt <= kc.config.MaxProcessingRetries; attempt++ {
		if ctx.Err() != nil {
			kc.logger.Warn().Msg("Context canceled during message processing")
			return
		}

		if err := handler(ctx, &msg); err != nil {
			kc.logger.Warn().
				Err(err).
				Int("attempt", attempt).
				Msg("Handler failed for message")
			// Exponantial backoff between attempts.
			time.Sleep(time.Second * time.Duration(1<<attempt))
			continue
		}
		success = true
		break
	}

	// If the message was not successfully processed after retries...
	if !success {
		kc.logger.Error().
			Str("topic", msg.Topic).
			Msg("Message processing failed after retries")

		// If a Dead Letter Producer is provided, send the failed message to the DLQ.
		if !utils.IsNilEmptyOrWhitespace(&kc.config.DeadLetterTopic) {

			// Optionally, you can add headers (e.g. original topic, timestamp) similar to the producer's DLQ.
			if err := kc.deadLetterProducer.Produce(ctx, kc.config.DeadLetterTopic, msg.Key, msg.Value); err != nil {
				kc.logger.Error().
					Err(err).
					Msg("Failed to send message to DLQ")
			} else {
				kc.logger.Info().Msg("Message sent to DLQ")
			}
		}
	} else {
		// On successful processing, commit the message offset if using manual commits.
		// When CommitInterval > 0, offsets are auto-committed.
		if kc.config.CommitInterval == 0 {
			if err := kc.reader.CommitMessages(ctx, msg); err != nil {
				kc.logger.Error().Err(err).Msg("Failed to commit message offset")
			}
		}
	}
}

// Close gracefully shuts down the Kafka consumer.
func (kc *kafkaConsumer) Close() error {
	var err error
	kc.closeOnce.Do(func() {
		kc.closedMutex.Lock()
		if kc.closed {
			return
		}
		kc.closed = true
		kc.closedMutex.Unlock()

		// Close the message channel to stop processing goroutines.
		close(kc.msgChannel)

		// Wait for all message processing goroutines to finish.
		kc.wg.Wait()

		if err = kc.reader.Close(); err != nil {
			kc.logger.Error().Err(err).Msg("Error closing Kafka reader")
			return
		}
		kc.logger.Info().Msg("Kafka consumer closed successfully")
	})

	return err
}
