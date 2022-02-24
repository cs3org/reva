// Package config should be moved to internal
package config

import (
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
)

// Repo defines which storage implementation is to be used.
type Repo struct {
	Backend string
	Disk    Disk
	CS3     CS3
}

// Disk is the local disk implementation of the storage.
type Disk struct {
	Path string
}

// CS3 is the cs3 implementation of the storage.
type CS3 struct {
	ProviderAddr string
	DataURL      string
	DataPrefix   string
}

// Index defines config for indexes.
type Index struct {
	UID, GID Bound
}

// Bound defines a lower and upper bound.
type Bound struct {
	Lower, Upper int64
}

// Config merges all Account config parameters.
type Config struct {
	Repo        Repo
	Index       Index
	ServiceUser *user.User
}

// New returns a new config.
func New() *Config {
	return &Config{}
}
