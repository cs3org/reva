package registry

import (
	"reflect"
	"testing"
)

func TestParseConfig(t *testing.T) {
	type args struct {
		m map[string]interface{}
	}
	tests := []struct {
		name    string
		args    args
		want    *Config
		wantErr bool
	}{
		{name: "parse config", args: args{map[string]interface{}{
			"services": map[string]map[string]interface{}{
				"authprovider": map[string]interface{}{
					"basic": map[string]interface{}{
						"name": "auth-basic",
						"nodes": []map[string]interface{}{
							{
								"address":  "0.0.0.0:1234",
								"metadata": map[string]string{"version": "v0.1.0"},
							},
						},
					},
					"bearer": map[string]interface{}{
						"name": "auth-bearer",
						"nodes": []map[string]interface{}{
							{
								"address":  "0.0.0.0:5678",
								"metadata": map[string]string{"version": "v0.1.0"},
							},
						},
					},
				},
			},
		}}, want: &Config{
			Services: map[string]map[string]*service{
				"authprovider": map[string]*service{
					"basic": &service{
						Name: "auth-basic",
						Nodes: []node{{
							Address:  "0.0.0.0:1234",
							Metadata: map[string]string{"version": "v0.1.0"},
						}},
					},
					"bearer": &service{
						Name: "auth-bearer",
						Nodes: []node{{
							Address:  "0.0.0.0:5678",
							Metadata: map[string]string{"version": "v0.1.0"},
						}},
					},
				},
			},
		}, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseConfig(tt.args.m)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseConfig() got = %v, want %v", got, tt.want)
			}
		})
	}
}
