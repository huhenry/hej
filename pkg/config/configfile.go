package config

import (
	"flag"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

const (
	configFile = "config-file"
)

// AddConfigFileFlag adds flags for ExternalConfFlags
func AddConfigFileFlag(flagSet *flag.FlagSet) {
	flagSet.String(configFile, "", "Configuration file in JSON, TOML, YAML, HCL, or Java properties formats (default none). See spf13/viper for precedence.")
}

// TryLoadConfigFile initializes viper with config file specified as flag
func TryLoadConfigFile(v *viper.Viper) (*BasicConfig, error) {
	c := &BasicConfig{
		Viper: v,
	}

	if file := v.GetString(configFile); file != "" {
		v.SetConfigFile(file)
		err := v.ReadInConfig()
		if err != nil {
			return nil, errors.Wrapf(err, "Error loading config file %s", file)
		}
	}
	return c, nil
}
