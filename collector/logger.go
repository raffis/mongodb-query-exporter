package collector

// Logger interface which is used in the collector
// You may use a custom loggger implementing this interface and pass to the collector
// with Collector.WithLogger(logger)
type Logger interface {
	Debugf(msg string, keysAndValues ...interface{})
	Infof(msg string, keysAndValues ...interface{})
	Errorf(msg string, keysAndValues ...interface{})
	Warnf(msg string, keysAndValues ...interface{})
	Fatalf(msg string, keysAndValues ...interface{})
	Panicf(msg string, keysAndValues ...interface{})
}

type dummyLogger struct{}

func (*dummyLogger) Debugf(msg string, keysAndValues ...interface{}) {}
func (*dummyLogger) Infof(msg string, keysAndValues ...interface{})  {}
func (*dummyLogger) Errorf(msg string, keysAndValues ...interface{}) {}
func (*dummyLogger) Warnf(msg string, keysAndValues ...interface{})  {}
func (*dummyLogger) Fatalf(msg string, keysAndValues ...interface{}) {}
func (*dummyLogger) Panicf(msg string, keysAndValues ...interface{}) {}
