package config

import (
	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
	"github.com/spf13/viper"
)

type Config struct {
	Server       ServerConfig       `mapstructure:"server"`
	Kafka        KafkaConfig        `mapstructure:"kafka"`
	Notification NotificationConfig `mapstructure:"notifications"`
}

type KafkaConfig struct {
	BootstrapServers string `mapstructure:"bootstrap-servers"`
	RequiredAcks     int    `mapstructure:"required-acks"`
	MaxRetries       int    `mapstructure:"max-retries"`
}

type ServerConfig struct {
	Port string
}

type NotificationConfig struct {
	ChannelTopics   map[string]string `mapstructure:"channel-topics"`
	DeadLetterTopic string            `mapstructure:"dead-letter-topic"`
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
	viper.AddConfigPath("../internal/config") // Adjust if your config is in a different directory

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
	var config Config
	err := viper.Unmarshal(&config)
	if err != nil {
		logger.Err(err).Msg("Error unmarshalling kafka config")
		panic(err)
	}

	appConfig = config
	logger.Info().Msg("Configuration updated")
}
