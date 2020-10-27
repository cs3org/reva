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

package net_test

import (
	"fmt"
	"strings"
	"testing"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"

	"github.com/cs3org/reva/pkg/sdk/common"
	"github.com/cs3org/reva/pkg/sdk/common/crypto"
	"github.com/cs3org/reva/pkg/sdk/common/net"
	testintl "github.com/cs3org/reva/pkg/sdk/common/testing"
)

type rpcStatusTest struct {
	status rpc.Code
}

func (r *rpcStatusTest) GetStatus() *rpc.Status {
	return &rpc.Status{
		Code: r.status,
	}
}

func TestCheckRPCInvocation(t *testing.T) {
	tests := []struct {
		operation     string
		status        rpcStatusTest
		shouldSucceed bool
		callError     error
	}{
		{"ok-check", rpcStatusTest{rpc.Code_CODE_OK}, true, nil},
		{"fail-status", rpcStatusTest{rpc.Code_CODE_NOT_FOUND}, false, nil},
		{"fail-err", rpcStatusTest{rpc.Code_CODE_OK}, false, fmt.Errorf("failed")},
	}

	for _, test := range tests {
		err := net.CheckRPCInvocation(test.operation, &test.status, test.callError)
		if err != nil && test.shouldSucceed {
			t.Errorf(testintl.FormatTestError("CheckRPCInvocation", err, test.operation, test.status, test.callError))
		} else if err == nil && !test.shouldSucceed {
			t.Errorf(testintl.FormatTestError("CheckRPCInvocation", fmt.Errorf("accepted an invalid RPC invocation"), test.operation, test.status, test.callError))
		}
	}
}

func TestTUSClient(t *testing.T) {
	tests := []struct {
		endpoint      string
		shouldSucceed bool
	}{
		{"https://tusd.tusdemo.net/files/", true},
		{"https://google.de", false},
	}

	for _, test := range tests {
		t.Run(test.endpoint, func(t *testing.T) {
			if client, err := net.NewTUSClient(test.endpoint, "", ""); err == nil {
				data := strings.NewReader("This is a simple TUS test")
				dataDesc := common.CreateDataDescriptor("tus-test.txt", data.Size())
				checksumTypeName := crypto.GetChecksumTypeName(provider.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_MD5)

				if err := client.Write(data, dataDesc.Name(), &dataDesc, checksumTypeName, ""); err != nil && test.shouldSucceed {
					t.Errorf(testintl.FormatTestError("TUSClient.Write", err, data, dataDesc.Name(), &dataDesc, checksumTypeName, ""))
				} else if err == nil && !test.shouldSucceed {
					t.Errorf(testintl.FormatTestError("TUSClient.Write", fmt.Errorf("writing to a non-TUS host succeeded"), data, dataDesc.Name(), &dataDesc, checksumTypeName, ""))
				}
			} else {
				t.Errorf(testintl.FormatTestError("NewTUSClient", err, test.endpoint, "", ""))
			}
		})
	}
}

func TestWebDAVClient(t *testing.T) {
	tests := []struct {
		endpoint      string
		shouldSucceed bool
	}{
		{"https://zivowncloud2.uni-muenster.de/owncloud/remote.php/dav/files/testUser/", true},
		{"https://google.de", false},
	}

	for _, test := range tests {
		t.Run(test.endpoint, func(t *testing.T) {
			if client, err := net.NewWebDAVClient(test.endpoint, "testUser", "test12345"); err == nil {
				const fileName = "webdav-test.txt"

				data := strings.NewReader("This is a simple WebDAV test")
				if err := client.Write(fileName, data, data.Size()); err == nil {
					if test.shouldSucceed {
						if _, err := client.Read(fileName); err != nil {
							t.Errorf(testintl.FormatTestError("WebDAVClient.Read", err))
						}

						if err := client.Remove(fileName); err != nil {
							t.Errorf(testintl.FormatTestError("WebDAVClient.Remove", err))
						}
					} else {
						t.Errorf(testintl.FormatTestError("WebDAVClient.Write", fmt.Errorf("writing to a non-WebDAV host succeeded"), fileName, data, data.Size()))
					}
				} else if test.shouldSucceed {
					t.Errorf(testintl.FormatTestError("WebDAVClient.Write", err, fileName, data, data.Size()))
				}
			} else {
				t.Errorf(testintl.FormatTestError("NewWebDavClient", err, test.endpoint, "testUser", "test12345"))
			}
		})
	}
}

func TestHTTPRequest(t *testing.T) {
	tests := []struct {
		url           string
		shouldSucceed bool
	}{
		{"https://google.de", true},
		{"https://ujhwrgobniwoeo.de", false},
	}

	// Prepare the session
	if session, err := testintl.CreateTestSession("sciencemesh-test.uni-muenster.de:9600", "test", "testpass"); err == nil {
		for _, test := range tests {
			t.Run(test.url, func(t *testing.T) {
				if request, err := session.NewHTTPRequest(test.url, "GET", "", nil); err == nil {
					if _, err := request.Do(true); err != nil && test.shouldSucceed {
						t.Errorf(testintl.FormatTestError("HTTPRequest.Do", err))
					} else if err == nil && !test.shouldSucceed {
						t.Errorf(testintl.FormatTestError("HTTPRequest.Do", fmt.Errorf("send request to an invalid host succeeded")))
					}
				} else {
					t.Errorf(testintl.FormatTestError("Session.NewHTTPRequest", err, test.url, "GET", "", nil))
				}
			})
		}
	} else {
		t.Errorf(testintl.FormatTestError("CreateTestSession", err))
	}
}
