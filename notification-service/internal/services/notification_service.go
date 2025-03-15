package services

import (
	"context"

	"github.com/Givko/NotificationSystem/notification-service/internal/config"
	"github.com/Givko/NotificationSystem/notification-service/internal/infrastructure/kafka"
	"github.com/Givko/NotificationSystem/notification-service/internal/models"
	"github.com/rs/zerolog"
)

type NotificationService interface {
	SendNotification(notification models.Notification) error
}

var _ NotificationService = (*notificationService)(nil)

type notificationService struct {
	producer kafka.Producer
	logger   zerolog.Logger
}

func NewNotificationService(producer kafka.Producer, logger zerolog.Logger) NotificationService {
	return &notificationService{
		producer: producer,
		logger:   logger,
	}
}

func (service *notificationService) SendNotification(notification models.Notification) error {
	config := config.GetConfig()
	ctx := context.Background()
	messageJson, err := notification.ToJSON()
	if err != nil {
		service.logger.Error().Err(err).Msg("Failed to marshal notification to JSON")
		return err
	}

	// Get the correct notification topic based on the channel in the notiication

	errProduce := service.producer.Produce(ctx, config.NotificationTopic, []byte(notification.RecipientID), messageJson)
	if errProduce != nil {
		service.logger.Error().Err(err).Msg("Failed to produce message")
		return err
	}

	return nil
}
