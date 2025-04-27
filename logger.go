package netconf

type Logger interface {
	Printf(string, ...any)
	Debugf(string, ...any)
	Infof(string, ...any)
	Warnf(string, ...any)
	Errorf(string, ...any)
}

type noOpLogger struct{}

func (l *noOpLogger) Printf(_ string, _ ...any) {}
func (l *noOpLogger) Debugf(_ string, _ ...any) {}
func (l *noOpLogger) Infof(_ string, _ ...any)  {}
func (l *noOpLogger) Warnf(_ string, _ ...any)  {}
func (l *noOpLogger) Errorf(_ string, _ ...any) {}
