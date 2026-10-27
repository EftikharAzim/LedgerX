package observability

import "go.uber.org/zap"

// MustLogger returns the development or production logger for env.
func MustLogger(env string) *zap.Logger {
	if env == "dev" {
		l, _ := zap.NewDevelopment()
		return l
	}
	l, _ := zap.NewProduction()
	return l
}
