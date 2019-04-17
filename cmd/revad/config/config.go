// Copyright 2018-2019 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization 
// or submit itself to any jurisdiction.

package config

import (
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
			(*kv)[k] = v.Get(prefix + "." + k)
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
