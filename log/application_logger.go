package log

import "github.com/rs/zerolog"

type ApplicationLogger interface {
	Debug(msg string)
	Info(msg string)
	Warn(msg string)
	Error(msg string)
	Fatal(msg string)

	Debugf(msg string, v ...interface{})
	Warnf(msg string, v ...interface{})
	Infof(msg string, v ...interface{})
	Errorf(msg string, v ...interface{})
	Fatalf(msg string, v ...interface{})
}

func ApplicationLoggerFromZerolog(logger *zerolog.Logger) ApplicationLogger {
	return &ZerologApplicationLogger{
		Log: logger,
	}
}

type ZerologApplicationLogger struct {
	Log *zerolog.Logger
}

func (l *ZerologApplicationLogger) Debug(msg string) {
	l.Log.Debug().Msg(msg)
}
func (l *ZerologApplicationLogger) Info(msg string) {
	l.Log.Info().Msg(msg)
}
func (l *ZerologApplicationLogger) Warn(msg string) {
	l.Log.Warn().Msg(msg)
}
func (l *ZerologApplicationLogger) Error(msg string) {
	l.Log.Error().Msg(msg)
}
func (l *ZerologApplicationLogger) Fatal(msg string) {
	l.Log.Fatal().Msg(msg)
}
func (l *ZerologApplicationLogger) Debugf(msg string, v ...interface{}) {
	l.Log.Debug().Msgf(msg, v...)
}
func (l *ZerologApplicationLogger) Warnf(msg string, v ...interface{}) {
	l.Log.Warn().Msgf(msg, v...)
}
func (l *ZerologApplicationLogger) Infof(msg string, v ...interface{}) {
	l.Log.Info().Msgf(msg, v...)
}
func (l *ZerologApplicationLogger) Errorf(msg string, v ...interface{}) {
	l.Log.Error().Msgf(msg, v...)
}
func (l *ZerologApplicationLogger) Fatalf(msg string, v ...interface{}) {
	l.Log.Fatal().Msgf(msg, v...)
}
