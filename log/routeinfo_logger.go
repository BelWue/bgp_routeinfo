package log

import (
	gobgplog "github.com/osrg/gobgp/v3/pkg/log"
	zerolog "github.com/rs/zerolog/log"
)

type RouteinfoLogger interface {
	SetBgpLogger(log gobgplog.Logger)
	SetApplicationLogger(log ApplicationLogger)

	GetBgpLogger() gobgplog.Logger
	GetApplicationLogger() ApplicationLogger
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
		r.bgpLogger = &silentBGPLogger{}
	}
	return r.bgpLogger
}
