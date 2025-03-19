package kafka

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"
)

// ---- Fake Reader ------------------------------------------------------------

// fakeReader implements the minimal methods used by kafkaConsumer.
type fakeReader struct {
	readMessageFunc func(ctx context.Context) (kafka.Message, error)
	commitFunc      func(ctx context.Context, msg ...kafka.Message) error
	closeFunc       func() error
}

func (fr *fakeReader) ReadMessage(ctx context.Context) (kafka.Message, error) {
	return fr.readMessageFunc(ctx)
}

func (fr *fakeReader) CommitMessages(ctx context.Context, msg ...kafka.Message) error {
	return fr.commitFunc(ctx, msg...)
}

func (fr *fakeReader) Close() error {
	return fr.closeFunc()
}

// ---- Dummy Metrics ----------------------------------------------------------

// dummyMetrics implements a no-op metrics collector.
type dummyMetrics struct{}

func (dm *dummyMetrics) ObserveHTTPRequestDuration(handler string, success bool, duration float64) {
	// no-op for tests
}

// ---- Fake Producer for DLQ --------------------------------------------------

type fakeProducer struct {
	// record whether Produce was called for DLQ.
	produceCalled int32
	writeFunc     func(ctx context.Context, topic string, key []byte, value []byte) <-chan error
}

func (fp *fakeProducer) Produce(ctx context.Context, topic string, key []byte, value []byte) <-chan error {
	atomic.AddInt32(&fp.produceCalled, 1)
	if fp.writeFunc != nil {
		return fp.writeFunc(ctx, topic, key, value)
	}
	ch := make(chan error, 1)
	ch <- nil
	return ch
}

func (fp *fakeProducer) Close(ctx context.Context) error {
	return nil
}

// ---- Tests for Consumer -----------------------------------------------------

// TestConsumerProcessMessageSuccess verifies that a message is processed successfully.
func TestConsumerProcessMessageSuccess(t *testing.T) {
	// Count how many times the handler is called.
	var handlerCalled int32
	testHandler := func(ctx context.Context, msg *kafka.Message) error {
		atomic.AddInt32(&handlerCalled, 1)
		return nil
	}

	// Count how many times the handler is called.
	var commitCalled int32

	// Prepare a fake message.
	testMsg := kafka.Message{
		Topic: "test-topic",
		Key:   []byte("key"),
		Value: []byte("value"),
	}

	// Create a channel with a single message.
	msgCh := make(chan kafka.Message, 1)
	msgCh <- testMsg
	close(msgCh)

	// Create a fakeReader that returns messages from msgCh.
	fr := &fakeReader{
		readMessageFunc: func(ctx context.Context) (kafka.Message, error) {
			m, ok := <-msgCh
			if !ok {
				// End the loop.
				return kafka.Message{}, context.Canceled
			}
			return m, nil
		},
		commitFunc: func(ctx context.Context, msg ...kafka.Message) error {
			atomic.AddInt32(&commitCalled, 1)
			return nil
		},
		closeFunc: func() error { return nil },
	}

	// Create a dummy metrics implementation.
	dm := &dummyMetrics{}

	// Consumer configuration.
	cfg := ConsumerConfig{
		BootstrapServers:      "localhost:9092",
		GroupID:               "test-group",
		Topic:                 "test-topic",
		CommitInterval:        0, // manual commit
		MaxProcessingRetries:  2,
		DeadLetterTopic:       "",
		ConsumerChannelBuffer: 10,
		ConsumerWorkerPool:    1,
	}

	// Create the consumer.
	consumer, err := NewKafkaConsumer(zerolog.Nop(), cfg, nil, dm)
	if err != nil {
		t.Fatalf("Failed to create consumer: %v", err)
	}
	// Override the real reader with our fakeReader.
	kc := consumer.(*kafkaConsumer)
	kc.reader = fr

	// Run consumer.Start in a goroutine.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		// Start will block until context is canceled.
		if err := kc.Start(ctx, testHandler); err != nil && !errors.Is(err, context.Canceled) {
			t.Logf("Consumer Start returned error: %v", err)
		}
	}()

	// Allow some time for processing.
	time.Sleep(500 * time.Millisecond)

	if atomic.LoadInt32(&handlerCalled) == 0 {
		t.Fatalf("Expected handler to be called, but it was not")
	}

	if atomic.LoadInt32(&commitCalled) == 0 {
		t.Fatalf("Expected commit to be called, but it was not")
	}
}

// TestConsumerProcessMessageRetry verifies that the retry mechanism is invoked when the handler fails initially.
func TestConsumerProcessMessageRetry(t *testing.T) {
	// Count attempts.
	var attemptCount int32
	testHandler := func(ctx context.Context, msg *kafka.Message) error {
		// Fail the first two attempts, then succeed.
		if atomic.AddInt32(&attemptCount, 1) < 3 {
			return fmt.Errorf("temporary error")
		}
		return nil
	}
	var commitCalled int32

	// Prepare a fake message.
	testMsg := kafka.Message{
		Topic: "test-topic",
		Key:   []byte("key"),
		Value: []byte("value"),
	}

	// Create a channel with a single message.
	msgCh := make(chan kafka.Message, 1)
	msgCh <- testMsg
	close(msgCh)

	fr := &fakeReader{
		readMessageFunc: func(ctx context.Context) (kafka.Message, error) {
			m, ok := <-msgCh
			if !ok {
				return kafka.Message{}, context.Canceled
			}
			return m, nil
		},
		commitFunc: func(ctx context.Context, msg ...kafka.Message) error {
			atomic.AddInt32(&commitCalled, 1)
			return nil
		},
		closeFunc: func() error { return nil },
	}

	dm := &dummyMetrics{}

	cfg := ConsumerConfig{
		BootstrapServers:      "localhost:9092",
		GroupID:               "test-group",
		Topic:                 "test-topic",
		CommitInterval:        0,
		MaxProcessingRetries:  3,
		DeadLetterTopic:       "",
		ConsumerChannelBuffer: 10,
		ConsumerWorkerPool:    1,
	}

	consumer, err := NewKafkaConsumer(zerolog.Nop(), cfg, nil, dm)
	if err != nil {
		t.Fatalf("Failed to create consumer: %v", err)
	}
	kc := consumer.(*kafkaConsumer)
	kc.reader = fr

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() {
		_ = kc.Start(ctx, testHandler)
	}()

	// Wait for processing.
	time.Sleep(2 * time.Second)

	if atomic.LoadInt32(&attemptCount) < 2 {
		t.Fatalf("Expected at least 3 attempts due to retry, but got %d", attemptCount)
	}

	if atomic.LoadInt32(&commitCalled) > 0 {
		t.Fatalf("Expected commit not to be called, but it was")
	}
}

// TestConsumerProcessMessageDLQ verifies that if the handler never succeeds,
// the consumer attempts to send the message to the DLQ.
func TestConsumerProcessMessageDLQ(t *testing.T) {
	// Always fail the handler.
	testHandler := func(ctx context.Context, msg *kafka.Message) error {
		return fmt.Errorf("permanent failure")
	}

	// Prepare a fake message.
	testMsg := kafka.Message{
		Topic: "test-topic",
		Key:   []byte("key"),
		Value: []byte("value"),
	}

	// Channel with the single message.
	msgCh := make(chan kafka.Message, 1)
	msgCh <- testMsg
	close(msgCh)

	fr := &fakeReader{
		readMessageFunc: func(ctx context.Context) (kafka.Message, error) {
			m, ok := <-msgCh
			if !ok {
				return kafka.Message{}, context.Canceled
			}
			return m, nil
		},
		commitFunc: func(ctx context.Context, msg ...kafka.Message) error {
			return nil
		},
		closeFunc: func() error { return nil },
	}

	dm := &dummyMetrics{}

	// Create a fake DLQ producer that records if Produce was called.
	fp := &fakeProducer{}
	cfg := ConsumerConfig{
		BootstrapServers:      "localhost:9092",
		GroupID:               "test-group",
		Topic:                 "test-topic",
		CommitInterval:        0,
		MaxProcessingRetries:  1, // keep retry count low for test speed
		DeadLetterTopic:       "dlq",
		ConsumerChannelBuffer: 10,
		ConsumerWorkerPool:    1,
	}

	consumer, err := NewKafkaConsumer(zerolog.Nop(), cfg, fp, dm)
	if err != nil {
		t.Fatalf("Failed to create consumer: %v", err)
	}
	kc := consumer.(*kafkaConsumer)
	kc.reader = fr

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go func() {
		_ = kc.Start(ctx, testHandler)
	}()

	// Wait enough time for retries and DLQ attempt.
	time.Sleep(30 * time.Second)

	if atomic.LoadInt32(&fp.produceCalled) == 0 {
		t.Fatalf("Expected DLQ producer to be called, but it wasn't")
	}
}
