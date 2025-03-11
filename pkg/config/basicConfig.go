package config

import (
	"time"

	"github.com/spf13/cast"
	"github.com/spf13/viper"
)

type BasicConfig struct {
	Viper *viper.Viper
}

func (c *BasicConfig) Get(key string) interface{} {
	return c.Viper.Get(key)
}

func (c *BasicConfig) IsSet(key string) bool {
	return c.Viper.IsSet(key)
}

func (c *BasicConfig) GetBool(key string) bool {
	return c.Viper.GetBool(key)
}

func (c *BasicConfig) GetFloat64(key string) float64 {
	return c.Viper.GetFloat64(key)
}

func (c *BasicConfig) GetInt(key string) int {
	return c.Viper.GetInt(key)
}

func (c *BasicConfig) GetIntSlice(key string) []int {
	return cast.ToIntSlice(c.Viper.Get(key))
}

func (c *BasicConfig) GetString(key string) string {
	return c.Viper.GetString(key)
}

func (c *BasicConfig) GetStringMap(key string) map[string]interface{} {
	return c.Viper.GetStringMap(key)
}

func (c *BasicConfig) GetStringMapString(key string) map[string]string {
	return c.Viper.GetStringMapString(key)
}

func (c *BasicConfig) GetStringSlice(key string) []string {
	return c.Viper.GetStringSlice(key)
}

func (c *BasicConfig) GetTime(key string) time.Time {
	return c.Viper.GetTime(key)
}

func (c *BasicConfig) GetAllConfig() map[string]interface{} {
	return c.Viper.AllSettings()
}

func (c *BasicConfig) GetDuration(key string) time.Duration {
	return c.Viper.GetDuration(key)
}
