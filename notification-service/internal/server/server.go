package server

import (
	"os"

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

// Server is the server struct
func InitServer() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs

	var zl zerolog.Logger
	if os.Getenv("ENV") == "dev" {
		zl = zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05",
		}).With().Timestamp().Logger()
	} else {
		//Log into file in order for the logs to be persisted\
		file, err := os.OpenFile("logs.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			panic(err)
		}
		zl = zerolog.New(file).With().Timestamp().Logger()
	}

	server := gin.Default()
	config.Init(zl)

	configuration := config.GetConfig()
	producer, err := kafka.NewKafkaProducer(zl, kafka.Config{
		BootstrapServers: configuration.Kafka.BootstrapServers,
		DeadLetterTopic:  configuration.Notification.DeadLetterTopic,
		MaxRetries:       configuration.Kafka.MaxRetries,
		RequiredAcks:     configuration.Kafka.RequiredAcks,
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
	server.Run(":" + configuration.Server.Port)
}
