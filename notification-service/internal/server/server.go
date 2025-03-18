package server

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Givko/NotificationSystem/notification-service/internal/config"
	"github.com/Givko/NotificationSystem/notification-service/internal/handlers"
	"github.com/Givko/NotificationSystem/notification-service/internal/infrastructure/kafka"
	"github.com/Givko/NotificationSystem/notification-service/internal/infrastructure/middlewares"
	"github.com/Givko/NotificationSystem/notification-service/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"

	"github.com/Givko/NotificationSystem/notification-service/internal/infrastructure/metrics"
)

func InitServer() {
	signalContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	var zl zerolog.Logger
	if os.Getenv("ENV") == "dev" {
		zl = zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05",
		}).With().Timestamp().Logger()
	} else {
		// Production: Use JSON format to stdout (machine-readable)
		zl = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}

	server := gin.Default()
	config.Init(zl)

	configuration := config.GetConfig()
	producer, err := kafka.NewKafkaProducer(zl, kafka.Config{
		BootstrapServers:     configuration.Kafka.BootstrapServers,
		DeadLetterTopic:      configuration.Notification.DeadLetterTopic,
		MaxRetries:           configuration.Kafka.MaxRetries,
		RequiredAcks:         configuration.Kafka.RequiredAcks,
		MessageChannelBuffer: configuration.Kafka.MessageChannelBuffer,
	})

	if err != nil {
		zl.Error().Err(err).Msg("Failed to create Kafka producer")
		panic(err)
	}

	metrics := metrics.NewMetrics()
	server.Use(middlewares.MetricsMiddleware(metrics))

	notificationService := services.NewNotificationService(producer, zl)
	notificaitonHandler := handlers.NewNotificationHandler(notificationService, zl)
	notificationsGroup := server.Group("/api/v1")
	notificationsGroup.POST("/notifications", notificaitonHandler.CreateNotificationHandler)
	server.GET("/metrics", gin.WrapH(promhttp.Handler()))

	//If using k8s we can split this into liveness and readiness probes
	server.GET("/health", func(c *gin.Context) {

		//Here we will add some health checks for hard dependencies like DB, Kafka, etc.
		//For now, we will just return ok
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})

	srv := &http.Server{
		Addr:    ":" + configuration.Server.Port,
		Handler: server,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zl.Error().Err(err).Msg("Failed to start server")
			stop()
		}
	}()

	<-signalContext.Done()

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Graceful shutdown sequence
	zl.Info().Msg("Starting graceful shutdown...")

	// 1. Shutdown HTTP server
	if err := srv.Shutdown(shutdownCtx); err != nil {
		zl.Error().Err(err).Msg("Failed to shutdown server gracefully")
	}

	// 2. Close Kafka producer
	if producer != nil {
		closeProducerErr := producer.Close(shutdownCtx)
		if closeProducerErr != nil {
			zl.Error().Err(closeProducerErr).Msg("Failed to close Kafka producer")
		} else {
			zl.Info().Msg("Kafka producer closed")
		}
	}

	zl.Info().Msg("Shutdown complete")
}
