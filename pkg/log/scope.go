package log

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

type Scope struct {
	name        string
	outputLevel logrus.Level
	logger      *logrus.Logger
	loggerEntry *logrus.Entry
}

func (s *Scope) SetOutputLevel(level logrus.Level) {
	s.outputLevel = level
	s.logger.SetLevel(level)
}

func (s *Scope) GetOutputLevel() string {
	return s.outputLevel.String()
}

var (
	scopes = make(map[string]*Scope)
	lock   = sync.RWMutex{}
)

func RegisterScope(name string) *Scope {
	lock.Lock()
	defer lock.Unlock()

	s, ok := scopes[name]
	if !ok {
		s = &Scope{
			name:   name,
			logger: logrus.New(),
		}
		formatter := &ConsoleFormatter{}
		s.logger.SetFormatter(formatter)
		s.logger.SetOutput(os.Stdout)
		loggerEntry := s.logger.WithField("scope", name)
		s.loggerEntry = loggerEntry
		s.SetOutputLevel(loggingFlags.Level)
		scopes[name] = s
	}
	return s
}

func updateScope(name string, level string) error {
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		return err
	}
	lock.RLock()
	defer lock.RUnlock()
	if scope, ok := scopes[name]; ok {
		scope.SetOutputLevel(lvl)
	}
	return nil
}

func UpdateScopes(s string) error {
	level, err := logrus.ParseLevel(s)
	if err == nil {
		for _, scope := range Scopes() {
			scope.SetOutputLevel(level)
		}
		return nil
	}
	items := strings.Split(s, ",")
	for _, item := range items {
		ss := strings.Split(item, ":")
		if len(ss) != 2 {
			return fmt.Errorf("format of scope level must be <scope>:<level>")
		}
		if err := updateScope(ss[0], ss[1]); err != nil {
			return err
		}
	}
	return nil
}

func Scopes() map[string]*Scope {
	lock.RLock()
	defer lock.RUnlock()
	s := make(map[string]*Scope, len(scopes))
	for k, v := range scopes {
		s[k] = v
	}
	return s
}
