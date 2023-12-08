package netconf

type Logger interface {
	Printf(string, ...interface{})
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Warnf(string, ...interface{})
	Errorf(string, ...interface{})
}

type noOpLogger struct{}

func (l *noOpLogger) Printf(_ string, _ ...interface{}) {}
func (l *noOpLogger) Debugf(_ string, _ ...interface{}) {}
func (l *noOpLogger) Infof(_ string, _ ...interface{})  {}
func (l *noOpLogger) Warnf(_ string, _ ...interface{})  {}
func (l *noOpLogger) Errorf(_ string, _ ...interface{}) {}
