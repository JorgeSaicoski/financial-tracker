// Package logger is a thin wrapper around the standard logger so call
// sites depend on an interface instead of the global log package, which
// keeps them free to swap in structured logging later.
package logger

import (
	"log"
	"os"
)

type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
}

type stdLogger struct {
	l *log.Logger
}

func New() Logger {
	return &stdLogger{l: log.New(os.Stdout, "", log.LstdFlags)}
}

func (s *stdLogger) Info(msg string, args ...any) {
	s.l.Printf("INFO "+msg, args...)
}

func (s *stdLogger) Error(msg string, args ...any) {
	s.l.Printf("ERROR "+msg, args...)
}
