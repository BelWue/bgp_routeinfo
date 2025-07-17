package log

import (
	"github.com/rs/zerolog/log"
	zerolog "github.com/rs/zerolog/log"
)

type RouteinfoLogger interface {
	SetBgpLogger(log gobgplog.Logger)
	SetApplicationLogger(log ApplicationLogger)

	GetBgpLogger() gobgplog.Logger
	GetApplicationLogger() ApplicationLogger
	SetLogLevel(logLevel *string)
	DisableBgpLog()
}

type DefaultRouteInfoLogger struct {
	applicationLogger ApplicationLogger
	bgpLogger         gobgplog.Logger
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

func (r *DefaultRouteInfoLogger) SetBgpLogger(log gobgplog.Logger) {
	r.bgpLogger = log
}

func (r *DefaultRouteInfoLogger) GetBgpLogger() gobgplog.Logger {
	if r.bgpLogger == nil {
		r.bgpLogger = gobgplog.NewDefaultLogger()
	}
	return r.bgpLogger
}

func (r *DefaultRouteInfoLogger) SetLogLevel(logLevel *string) {
	r.GetApplicationLogger().SetLogLevel(logLevel)
	r.GetBgpLogger().SetLevel(GobgpLogLevel(logLevel))
}
func (r *DefaultRouteInfoLogger) DisableBgpLog() {
	r.bgpLogger = &silentBGPLogger{}
}
func GobgpLogLevel(logLevel *string) gobgplog.LogLevel {
	if logLevel != nil && *logLevel != "" {
		switch *logLevel {
		case "trace":
			return gobgplog.TraceLevel
		case "debug":
			return gobgplog.DebugLevel
		case "info":
			return gobgplog.InfoLevel
		case "warning":
			return gobgplog.WarnLevel
		case "error":
			return gobgplog.ErrorLevel
		case "fatal":
			return gobgplog.FatalLevel
		case "panic":
			return gobgplog.PanicLevel
		default:
			log.Warn().Msgf("Unknown log level '%s' returning default 'info'", *logLevel)
		}
	} else {
		log.Info().Msg("Empty log level - Returning default log level 'info'")
	}

	return gobgplog.InfoLevel
}
