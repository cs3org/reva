package config

import (
	"testing"

	"gotest.tools/assert"
)

func TestSplit(t *testing.T) {
	tests := []struct {
		key   string
		token string
		next  string
	}{
		{
			key:   ".grpc.services.authprovider[1].address",
			token: "grpc",
			next:  ".services.authprovider[1].address",
		},
		{
			key:   "[1].address",
			token: "1",
			next:  ".address",
		},
		{
			key:   "[100].address",
			token: "100",
			next:  ".address",
		},
		{
			key: "",
		},
	}

	for _, tt := range tests {
		token, next := split(tt.key)
		assert.Equal(t, token, tt.token)
		assert.Equal(t, next, tt.next)
	}
}

func TestParseNext(t *testing.T) {
	tests := []struct {
		key  string
		cmd  Command
		next string
		err  error
	}{
		{
			key:  ".grpc.services.authprovider[1].address",
			cmd:  FieldByKey{Key: "grpc"},
			next: ".services.authprovider[1].address",
		},
		{
			key:  ".authprovider[1].address",
			cmd:  FieldByKey{Key: "authprovider"},
			next: "[1].address",
		},
		{
			key:  "[1].authprovider.address",
			cmd:  FieldByIndex{Index: 1},
			next: ".authprovider.address",
		},
		{
			key:  ".authprovider",
			cmd:  FieldByKey{Key: "authprovider"},
			next: "",
		},
	}

	for _, tt := range tests {
		cmd, next, err := parseNext(tt.key)
		assert.Equal(t, err, tt.err)
		if tt.err == nil {
			assert.Equal(t, cmd, tt.cmd)
			assert.Equal(t, next, tt.next)
		}
	}
}
