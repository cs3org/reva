package config

import (
	"github.com/spf13/viper"
)

var v *viper.Viper

func init() {
	v = viper.New()
}

func SetFile(fn string) {
	v.SetConfigFile(fn)
}

func Read() error {
	return v.ReadInConfig()
}

func Get(key string) map[string]interface{} {
	return v.GetStringMap(key)
}

func Dump() map[string]interface{} {
	return v.AllSettings()
}
