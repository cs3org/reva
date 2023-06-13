package config

import (
	"reflect"
	"testing"

	"github.com/gdexlab/go-render/render"
)

func TestSplit(t *testing.T) {
	tests := []struct {
		key      string
		exptoken string
		expnext  string
	}{
		{
			key:      ".grpc.services.authprovider[1].address",
			exptoken: "grpc",
			expnext:  ".services.authprovider[1].address",
		},
		{
			key:      "[1].address",
			exptoken: "1",
			expnext:  ".address",
		},
		{
			key:      "[100].address",
			exptoken: "100",
			expnext:  ".address",
		},
		{
			key: "",
		},
	}

	for _, tt := range tests {
		token, next := split(tt.key)
		if token != tt.exptoken || next != tt.expnext {
			t.Fatalf("unexpected result: token=%s exp=%s | next=%s exp=%s", token, tt.exptoken, next, tt.expnext)
		}
	}
}

func TestParseNext(t *testing.T) {
	tests := []struct {
		key     string
		expcmd  Command
		expnext string
		experr  error
	}{
		{
			key:     ".grpc.services.authprovider[1].address",
			expcmd:  FieldByKey{Key: "grpc"},
			expnext: ".services.authprovider[1].address",
		},
		{
			key:     ".authprovider[1].address",
			expcmd:  FieldByKey{Key: "authprovider"},
			expnext: "[1].address",
		},
		{
			key:     "[1].authprovider.address",
			expcmd:  FieldByIndex{Index: 1},
			expnext: ".authprovider.address",
		},
		{
			key:     ".authprovider",
			expcmd:  FieldByKey{Key: "authprovider"},
			expnext: "",
		},
	}

	for _, tt := range tests {
		cmd, next, err := parseNext(tt.key)
		if err != tt.experr || !reflect.DeepEqual(cmd, tt.expcmd) || next != tt.expnext {
			t.Fatalf("unexpected result: err=%v exp=%v | cmd=%s exp=%s | next=%s exp=%s", err, tt.experr, render.AsCode(cmd), render.AsCode(tt.expcmd), next, tt.expnext)
		}
	}
}
