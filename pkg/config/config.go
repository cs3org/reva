package config

import (
	"encoding/json"
	"io/ioutil"
)

type Config struct {
	Network  string   `json:"network"`
	Address  string   `json:"address"`
	Services []string `json:"services"`

	AuthSVC struct {
		Driver  string      `json:"driver"`
		Options interface{} `json:"options"`
	} `json:"auth_svc"`

	StorageProviderSVC struct {
		TemporaryFolder string      `json:"temporary_folder"`
		Driver          string      `json:"driver"`
		Options         interface{} `json:"options"`
	} `json:"storage_provider_svc"`
}

func LoadFromFile(fn string) (*Config, error) {
	data, err := ioutil.ReadFile(fn)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
