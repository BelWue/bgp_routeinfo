package log

import (
	"log/slog"

	"github.com/rs/zerolog/log"
	zerolog "github.com/rs/zerolog/log"
)

type RouteinfoLogger interface {
	SetBgpLogger(log *slog.Logger)
	SetApplicationLogger(log ApplicationLogger)

	GetBgpLogger() (*slog.Logger, *slog.LevelVar)
	GetApplicationLogger() ApplicationLogger
	SetLogLevel(logLevel *string)
	DisableBgpLog()
}

type DefaultRouteInfoLogger struct {
	applicationLogger ApplicationLogger
	bgpLogger         *slog.Logger
	bgpLoggerLevel    *slog.LevelVar
}

func (r *DefaultRouteInfoLogger) SetApplicationLogger(log ApplicationLogger) {
	r.applicationLogger = log
}

func (r *DefaultRouteInfoLogger) GetApplicationLogger() ApplicationLogger {
	if r.applicationLogger == nil {
		//init default
		r.applicationLogger = ApplicationLoggerFromZerolog(&zerolog.Logger)
	}
	return r.applicationLogger
}

func (r *DefaultRouteInfoLogger) SetBgpLogger(log *slog.Logger) {
	r.bgpLogger = log
}

func (r *DefaultRouteInfoLogger) GetBgpLogger() (*slog.Logger, *slog.LevelVar) {
	if r.bgpLogger == nil {
		r.bgpLogger = slog.Default()
	}
	if r.bgpLoggerLevel == nil {
		l := &slog.LevelVar{}
		l.Set(slog.LevelInfo)
		r.bgpLoggerLevel = l
	}
	return r.bgpLogger, r.bgpLoggerLevel
}

func (r *DefaultRouteInfoLogger) SetLogLevel(logLevel *string) {
	r.GetApplicationLogger().SetLogLevel(logLevel)
	if r.bgpLoggerLevel == nil {
		r.bgpLoggerLevel = &slog.LevelVar{}
	}
	r.bgpLoggerLevel.Set(GobgpLogLevel(logLevel))
}
func (r *DefaultRouteInfoLogger) DisableBgpLog() {
	r.bgpLogger = slog.New(slog.DiscardHandler)
}

// slog doesnt support all levels of zerolog.
// This method returns the closest available setting
func GobgpLogLevel(logLevel *string) slog.Level {
	if logLevel != nil && *logLevel != "" {
		switch *logLevel {
		case "trace":
			return slog.LevelDebug
		case "debug":
			return slog.LevelDebug
		case "info":
			return slog.LevelInfo
		case "warning":
			return slog.LevelWarn
		case "error":
			return slog.LevelError
		case "fatal":
			return slog.LevelError
		case "panic":
			return slog.LevelError
		default:
			log.Warn().Msgf("Unknown log level '%s' returning default 'info'", *logLevel)
		}
	} else {
		log.Info().Msg("Empty log level - Returning default log level 'info'")
	}

	return slog.LevelInfo
}
