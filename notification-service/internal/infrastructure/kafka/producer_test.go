package kafka

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"
)

// fakeWriter is a test double that implements the minimal subset of kafka.Writer’s behavior.
type fakeWriter struct {
	writeFunc func(ctx context.Context, msgs ...kafka.Message) error
	closeFunc func() error
}

func (f *fakeWriter) WriteMessages(ctx context.Context, msgs ...kafka.Message) error {
	if f.writeFunc != nil {
		return f.writeFunc(ctx, msgs...)
	}
	return nil
}

func (f *fakeWriter) Close() error {
	if f.closeFunc != nil {
		return f.closeFunc()
	}
	return nil
}

// TestProducerProduceSuccess verifies that a successful write returns no error.
func TestProducerProduceSuccess(t *testing.T) {
	logger := zerolog.Nop()
	cfg := Config{
		BootstrapServers:     "localhost:9092",
		RequiredAcks:         1,
		MaxRetries:           3,
		DeadLetterTopic:      "dlq",
		MessageChannelBuffer: 10,
		WorkerPoolSize:       1,
		BatchSize:            1,
		BatchTimeoutMs:       150,
	}

	prod, err := NewKafkaProducer(logger, cfg)
	if err != nil {
		t.Fatalf("Failed to create producer: %v", err)
	}

	// Override the writer with a fakeWriter that always succeeds.
	fw := &fakeWriter{
		writeFunc: func(ctx context.Context, msgs ...kafka.Message) error {
			return nil
		},
	}
	p := prod.(*kafkaProducer)
	p.writer = fw

	ctx := context.Background()
	errChan := prod.Produce(ctx, "test-topic", []byte("key"), []byte("value"))
	select {
	case e := <-errChan:
		if e != nil {
			t.Fatalf("Expected no error, got %v", e)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timed out waiting for Produce to return")
	}
}

// TestProducerProduceRetry simulates a failure on the first write then success on the retry.
func TestProducerProduceRetry(t *testing.T) {
	logger := zerolog.Nop()
	cfg := Config{
		BootstrapServers:     "localhost:9092",
		RequiredAcks:         1,
		MaxRetries:           3,
		DeadLetterTopic:      "dlq",
		MessageChannelBuffer: 10,
		WorkerPoolSize:       1,
		BatchSize:            1,
		BatchTimeoutMs:       150,
	}

	prod, err := NewKafkaProducer(logger, cfg)
	if err != nil {
		t.Fatalf("Failed to create producer: %v", err)
	}

	var callCount int32
	fw := &fakeWriter{
		writeFunc: func(ctx context.Context, msgs ...kafka.Message) error {
			count := atomic.AddInt32(&callCount, 1)
			// Return an error on the first call, then succeed.
			if count < 2 {
				return errors.New("temporary error")
			}
			return nil
		},
	}
	p := prod.(*kafkaProducer)
	p.writer = fw

	ctx := context.Background()
	errChan := prod.Produce(ctx, "test-topic", []byte("key"), []byte("value"))
	select {
	case e := <-errChan:
		if e != nil {
			t.Fatalf("Expected success after retry, got error: %v", e)
		}
		if callCount < 2 {
			t.Fatalf("Expected at least one retry, but call count is %d", callCount)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timed out waiting for Produce to return")
	}
}

// TestProducerProduceContextCancelled tests that if the context is cancelled before the message is enqueued,
// the errChan returns the proper error.
func TestProducerProduceContextCancelled(t *testing.T) {
	logger := zerolog.Nop()
	cfg := Config{
		BootstrapServers:     "localhost:9092",
		RequiredAcks:         1,
		MaxRetries:           3,
		DeadLetterTopic:      "dlq",
		MessageChannelBuffer: 10,
		WorkerPoolSize:       1,
		BatchSize:            1,
		BatchTimeoutMs:       150,
	}

	prod, err := NewKafkaProducer(logger, cfg)
	if err != nil {
		t.Fatalf("Failed to create producer: %v", err)
	}

	// Create a context and cancel it immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	errChan := prod.Produce(ctx, "test-topic", []byte("key"), []byte("value"))
	select {
	case e := <-errChan:
		if !errors.Is(e, context.Canceled) {
			t.Fatalf("Expected context.Canceled error, got: %v", e)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timed out waiting for error from Produce due to cancelled context")
	}
}
