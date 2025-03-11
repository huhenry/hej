package log

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

var (
	Logger = RegisterScope("default")
)

func (s *Scope) Tracef(format string, args ...interface{}) { s.loggerEntry.Tracef(format, args...) }
func (s *Scope) Trace(args ...interface{})                 { s.loggerEntry.Trace(args...) }

func (s *Scope) Debugf(format string, args ...interface{}) { s.loggerEntry.Debugf(format, args...) }
func (s *Scope) Debug(args ...interface{})                 { s.loggerEntry.Debug(args...) }

func (s *Scope) Infof(format string, args ...interface{}) { s.loggerEntry.Infof(format, args...) }
func (s *Scope) Info(args ...interface{})                 { s.loggerEntry.Info(args...) }

func (s *Scope) Warnf(format string, args ...interface{}) { s.loggerEntry.Warnf(format, args...) }
func (s *Scope) Warn(args ...interface{})                 { s.loggerEntry.Warn(args...) }

func (s *Scope) Errorf(format string, args ...interface{}) { s.WithCaller().Errorf(format, args...) }
func (s *Scope) Error(args ...interface{})                 { s.WithCaller().Error(args...) }

func (s *Scope) Fatalf(format string, args ...interface{}) { s.loggerEntry.Fatalf(format, args...) }
func (s *Scope) Fatal(args ...interface{})                 { s.loggerEntry.Fatal(args...) }

func (s *Scope) WithCaller() *logrus.Entry {
	stack := callers()
	//s.loggerEntry.WithCallerAt()
	return s.loggerEntry.WithField("file", fmt.Sprintf("%+v", stack))

}
