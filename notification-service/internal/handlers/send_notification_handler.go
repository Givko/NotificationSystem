package handlers

import (
	"github.com/Givko/NotificationSystem/notification-service/internal/services"
	"github.com/Givko/NotificationSystem/notification-service/pkg/contracts"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type NotificationHandler struct {
	service services.NotificationService
	logger  zerolog.Logger
}

func NewNotificationHandler(service services.NotificationService, logger zerolog.Logger) *NotificationHandler {
	return &NotificationHandler{
		service: service,
		logger:  logger,
	}
}

func (handler *NotificationHandler) CreateNotificationHandler(c *gin.Context) {
	var notification contracts.Notification

	// Bind JSON body to createDto
	if err := c.ShouldBindJSON(&notification); err != nil {
		handler.logger.Error().Err(err).Msg("Failed to bind JSON body")
		c.JSON(400, gin.H{"error": "Failed to bind JSON body"})
		return
	}

	err := handler.service.SendNotification(notification)
	if err != nil {
		handler.logger.Error().Err(err).Msg("Failed to produce message")
		c.JSON(500, gin.H{"error": "Failed to produce message"})
		return
	}

	c.JSON(202, gin.H{"message": "Notification sent"}) // 202 Accepted
}
