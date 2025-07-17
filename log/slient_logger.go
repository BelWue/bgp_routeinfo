package log

type silentBGPLogger struct {
}

func (l *silentBGPLogger) Panic(msg string, fields gobgplog.Fields) {
}

func (l *silentBGPLogger) Fatal(msg string, fields gobgplog.Fields) {
}

func (l *silentBGPLogger) Error(msg string, fields gobgplog.Fields) {
}

func (l *silentBGPLogger) Warn(msg string, fields gobgplog.Fields) {
}

func (l *silentBGPLogger) Info(msg string, fields gobgplog.Fields) {
}

func (l *silentBGPLogger) Debug(msg string, fields gobgplog.Fields) {
}

func (l *silentBGPLogger) SetLevel(level gobgplog.LogLevel) {
}

func (l *silentBGPLogger) GetLevel() gobgplog.LogLevel {
	return gobgplog.PanicLevel
}
