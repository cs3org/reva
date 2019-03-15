package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

var v *viper.Viper

func init() {
	v = viper.New()
	v.SetEnvPrefix("reva")                             // will be uppercased automatically
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_")) // but check eg "REVA_CORE_MAX_CPUS" on Get("core.max_cpus")
	v.AutomaticEnv()                                   // automagically read env vars on Get calls
}

func SetFile(fn string) {
	v.SetConfigFile(fn)
}

func Read() error {
	err := v.ReadInConfig()
	return err
}

// reGet will recursively walk the given map and execute
// vipers Get method to allow overwriting config vars with
// env variables.
func reGet(prefix string, kv *map[string]interface{}) {
	for k, val := range *kv {
		if c, ok := val.(map[string]interface{}); ok {
			reGet(prefix+"."+k, &c)
		} else {
			newVal := v.Get(prefix + "." + k)
			fmt.Println("reGet", prefix, k, "val:", val, "->", newVal)
			if newVal != nil {
				(*kv)[k] = newVal
			}
		}
	}

}

func Get(key string) map[string]interface{} {
	kv := v.GetStringMap(key)
	// we need to try and get from env as well because vipers
	// GetStringMap does not execute the automatic Get mapping
	// of env vars
	reGet(key, &kv)
	return kv
}

func Dump() map[string]interface{} {
	return v.AllSettings()
}
