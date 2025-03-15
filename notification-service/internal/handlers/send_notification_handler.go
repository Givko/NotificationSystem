package handlers

import (
	"github.com/Givko/NotificationSystem/notification-service/internal/config"
	"github.com/Givko/NotificationSystem/notification-service/internal/infrastructure/kafka"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type NotificationHandler struct {
	producer kafka.Producer
	logger   zerolog.Logger
}

func NewNotificationHandler(producer kafka.Producer, logger zerolog.Logger) *NotificationHandler {
	return &NotificationHandler{
		producer: producer,
		logger:   logger,
	}
}

func (handler *NotificationHandler) CreateNotificationHandler(c *gin.Context) {
	config := config.GetConfig()
	err := handler.producer.Produce(c, config.NotificationTopic, []byte("key"), []byte("value"))
	if err != nil {
		handler.logger.Error().Err(err).Msg("Failed to produce message")
		c.JSON(500, gin.H{"error": "Failed to produce message"})
		return
	}

	c.JSON(202, gin.H{"message": "Notification sent"}) // 202 Accepted
}
