package cs3

import (
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
)

// Config represents cs3conf. Should be deprecated in favor of config.Config.
type Config struct {
	ProviderAddr string
	JWTSecret    string
	ServiceUser  *user.User
}
