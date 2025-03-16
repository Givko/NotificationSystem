package services

import (
	"context"
	"fmt"

	"github.com/Givko/NotificationSystem/notification-service/internal/config"
	"github.com/Givko/NotificationSystem/notification-service/internal/infrastructure/kafka"
	"github.com/Givko/NotificationSystem/notification-service/pkg/contracts"
	"github.com/rs/zerolog"
)

type NotificationService interface {
	SendNotification(ctx context.Context, notification contracts.Notification) error
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

func (service *notificationService) SendNotification(ctx context.Context, notification contracts.Notification) error {
	config := config.GetConfig()

	messageJson, err := notification.ToJSON()
	if err != nil {
		service.logger.
			Error().
			Err(err).
			Msg("Failed to marshal notification to JSON")
		return err
	}

	// Get the correct notification topic based on the channel in the notiication
	channelTopic, ok := config.Notification.ChannelTopics[notification.Channel]
	if !ok {
		service.logger.
			Error().
			Str("channel", notification.Channel).
			Str("channelTopics", fmt.Sprintf("%#v", config.Notification.ChannelTopics)).
			Msg("Channel not found")
		return fmt.Errorf("channel %s not found", notification.Channel)
	}

	errProduce := service.producer.Produce(ctx, channelTopic, []byte(notification.RecipientID), messageJson)
	if errProduce != nil {
		service.logger.
			Error().
			Err(err).
			Msg("Failed to produce message")
		return err
	}

	return nil
}
