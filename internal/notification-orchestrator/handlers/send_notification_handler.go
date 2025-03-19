package handlers

import (
	"github.com/Givko/NotificationSystem/internal/notification-orchestrator/contracts"
	"github.com/Givko/NotificationSystem/internal/notification-orchestrator/services"
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
	reqCtx := c.Request.Context()
	err := handler.service.SendNotification(reqCtx, notification)
	if err != nil {
		handler.logger.Error().Err(err).Msg("Failed to produce message")
		c.JSON(500, gin.H{"error": "Failed to produce message"})
		return
	}

	c.JSON(202, gin.H{"message": "Notification sent"}) // 202 Accepted
}
