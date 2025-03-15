package server

import (
	"os"

	"github.com/Givko/NotificationSystem/notification-service/internal/config"
	"github.com/Givko/NotificationSystem/notification-service/internal/handlers"
	"github.com/Givko/NotificationSystem/notification-service/internal/infrastructure/kafka"
	"github.com/Givko/NotificationSystem/notification-service/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
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
		BootstrapServers: configuration.BootstrapServers,
		DeadLetterTopic:  configuration.NotificationTopicDLQ,
		MaxRetries:       3,
		RequiredAcks:     1,
	})
	if err != nil {
		zl.Error().Err(err).Msg("Failed to create Kafka producer")
		panic(err)
	}

	notificationService := services.NewNotificationService(producer, zl)
	notificaitonHandler := handlers.NewNotificationHandler(notificationService, zl)
	notificationsGroup := server.Group("/api/v1")
	notificationsGroup.POST("/notifications", notificaitonHandler.CreateNotificationHandler)

	server.Run(":" + configuration.ServerPort)
}
