package config

import (
	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
	"github.com/spf13/viper"
)

type Config struct {
	ServerPort           string
	NotificationTopic    string
	NotificationTopicDLQ string
	BootstrapServers     string

	// Add other configuration fields here.
}

// Unexported configuration variable
var appConfig Config

// GetConfig returns a copy of the current configuration. This makes it safe to use in concurrent operations and
// makes it impossible to modify the configuration from outside the package.
func GetConfig() Config {
	return appConfig
}

// Init initializes the configuration and starts watching for changes.
func Init(logger zerolog.Logger) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".") // Adjust if your config is in a different directory

	if err := viper.ReadInConfig(); err != nil {
		logger.Err(err).Msg("Error reading config file")
		panic(err)
	}

	updateConfig(logger)

	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		logger.Info().Msg("Config file changed")
		updateConfig(logger)
	})
}

func updateConfig(logger zerolog.Logger) {
	appConfig = Config{
		ServerPort:           viper.GetString("server.port"),
		NotificationTopic:    viper.GetString("kafka.notifications.topic"),
		NotificationTopicDLQ: viper.GetString("kafka.notifications.dlq-topic"),
		// Update other fields as needed
	}

	logger.Info().Msg("Configuration updated")
}
