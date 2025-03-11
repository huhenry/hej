package config

import (
	"flag"

	"github.com/spf13/viper"
)

const (
	http_Addr    = "http.http_addr"
	backend_host = "client.backend_host"
)

type Flags struct {
	addr string
}

// AddFlags adds logging flag for loggingFlags
func AddBaseFlags(flagSet *flag.FlagSet) {
	flagSet.String(http_Addr, "8090", "http address.")
	flagSet.String(backend_host, "", "http address.")
}

// InitFromViper initializes loggingFlags with properties from viper
func InitBaseFromViper(v *viper.Viper) *BasicConfig {
	c := &BasicConfig{
		Viper: v,
	}
	return c
}
