package main

import (
	"os"

	"github.com/rs/zerolog"
)

func main() {
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

}
