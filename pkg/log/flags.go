package log

import (
	"flag"
	"fmt"

	"github.com/spf13/viper"

	"github.com/sirupsen/logrus"
)

const (
	logLevel = "log.level"
)

type LoggingFlags struct {
	Level logrus.Level
}

var loggingFlags = &LoggingFlags{
	Level: logrus.InfoLevel,
}

// AddFlags adds logging flag for loggingFlags
func AddFlags(flagSet *flag.FlagSet) {
	flagSet.String(logLevel, "info", "Minimal allowed log Level.")
}

// InitFromViper initializes loggingFlags with properties from viper
func InitFromViper(v *viper.Viper) *LoggingFlags {
	levelstring := v.GetString(logLevel)
	level, err := logrus.ParseLevel(levelstring)
	if err != nil {
		fmt.Printf("invalided log level %s, will be seted as info log level.", levelstring)
		return loggingFlags
	}

	loggingFlags.Level = level

	return loggingFlags
}
