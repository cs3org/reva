// Copyright 2018-2020 CERN
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

package grpctests

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	v1beta11 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/stretchr/testify/assert"
)

const grpcAddress = "localhost:19000"
const timeoutMs = 30000

func Test_service_GetUser(t *testing.T) {
	providers := []struct {
		name        string
		existingIdp string
	}{
		{
			name:        "json",
			existingIdp: "localhost:20080",
		},
		{
			name:        "demo",
			existingIdp: "http://localhost:9998",
		},
	}

	for _, tt := range providers {
		t.Run(tt.name, func(t *testing.T) {
			//start revad with the specific provider
			cmd := exec.Command("../cmd/revad/revad", "-c", "userproviders/"+tt.name+".toml")
			err := cmd.Start()

			if err != nil {
				t.Fatalf("Could not start revad! ERROR: %v", err)
			}

			//wait till port is open
			_ = waitForPort("open")

			//even the port is open the service might not be available yet
			time.Sleep(1 * time.Second)

			GetUser(t, tt.existingIdp)

			//kill revad
			err = cmd.Process.Signal(os.Kill)
			if err != nil {
				t.Fatalf("Could not kill revad! ERROR: %v", err)
			}
			_ = waitForPort("close")
		})
	}
}

func GetUser(t *testing.T, existingIdp string) {
	tests := []struct {
		name   string
		userID *userpb.UserId
		want   *userpb.GetUserResponse
	}{
		{
			name: "simple",
			userID: &userpb.UserId{
				Idp:      existingIdp,
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
			},
			want: &userpb.GetUserResponse{
				Status: &v1beta11.Status{
					Code: 1,
				},
				User: &userpb.User{
					Username:    "marie",
					Mail:        "marie@example.org",
					DisplayName: "Marie Curie",
					Groups: []string{
						"radium-lovers",
						"polonium-lovers",
						"physics-lovers",
					},
				},
			},
		},
		{
			name: "not-existing opaqueId",
			userID: &userpb.UserId{
				Idp:      existingIdp,
				OpaqueId: "doesnote-xist-4376-b307-cf0a8c2d0d9c",
			},
			want: &userpb.GetUserResponse{
				Status: &v1beta11.Status{
					Code: 15,
				},
			},
		},
		{
			name: "no opaqueId",
			userID: &userpb.UserId{
				Idp:      existingIdp,
				OpaqueId: "",
			},
			want: &userpb.GetUserResponse{
				Status: &v1beta11.Status{
					Code: 15,
				},
			},
		},
		{
			name: "not-existing idp",
			userID: &userpb.UserId{
				Idp:      "http://does-not-exist:12345",
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
			},
			want: &userpb.GetUserResponse{
				Status: &v1beta11.Status{
					Code: 15,
				},
			},
		},
		{
			name: "no idp",
			userID: &userpb.UserId{
				OpaqueId: "f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
			},
			want: &userpb.GetUserResponse{
				Status: &v1beta11.Status{
					Code: 1,
				},
				User: &userpb.User{
					Username:    "marie",
					Mail:        "marie@example.org",
					DisplayName: "Marie Curie",
					Groups: []string{
						"radium-lovers",
						"polonium-lovers",
						"physics-lovers",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			serviceClient, err := pool.GetUserProviderServiceClient(grpcAddress)
			if err != nil {
				t.Fatalf("cannot get UserProviderServiceClient! ERROR: %v", err)
			}

			userResp, err := serviceClient.GetUser(ctx, &userpb.GetUserRequest{
				UserId: tt.userID,
			})
			if err != nil {
				t.Fatalf("cannot get user! ERROR: %v", err)
			}
			assert.Equal(t, tt.want.Status.Code, userResp.Status.Code)
			if tt.want.User == nil {
				assert.Nil(t, userResp.User)
			} else {
				//make sure not to run into a nil pointer error
				if userResp.User == nil {
					t.Fatalf("no user in response %v", userResp)
				}
				assert.Equal(t, tt.want.User.Username, userResp.User.Username)
				assert.Equal(t, tt.want.User.Mail, userResp.User.Mail)
				assert.Equal(t, tt.want.User.DisplayName, userResp.User.DisplayName)
				assert.Equal(t, tt.want.User.Groups, userResp.User.Groups)
			}
		})
	}
}

func waitForPort(expectedStatus string) error {
	if expectedStatus != "open" && expectedStatus != "close" {
		return errors.New("status can only be 'open' or 'close'")
	}
	timoutCounter := 0
	for timoutCounter <= timeoutMs {
		conn, err := net.Dial("tcp", grpcAddress)
		if err == nil {
			_ = conn.Close()
			if expectedStatus == "open" {
				break
			}
		} else if expectedStatus == "close" {
			break
		}

		time.Sleep(1 * time.Millisecond)
		timoutCounter++
	}
	return nil
}
