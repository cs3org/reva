// Copyright 2018-2021 CERN
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

package nextcloud

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
)

// Response contains data for the Nextcloud mock server to respond
// and to switch to a new server state
type Response struct {
	code           int
	body           string
	newServerState string
}

const serverStateError = "ERROR"
const serverStateEmpty = "EMPTY"
const serverStateHome = "HOME"
const serverStateSubdir = "SUBDIR"
const serverStateNewdir = "NEWDIR"
const serverStateSubdirNewdir = "SUBDIR-NEWDIR"
const serverStateFileRestored = "FILE-RESTORED"
const serverStateGrantAdded = "GRANT-ADDED"
const serverStateGrantUpdated = "GRANT-UPDATED"
const serverStateRecycle = "RECYCLE"
const serverStateReference = "REFERENCE"
const serverStateMetadata = "METADATA"

var serverState = serverStateEmpty

var responses = map[string]Response{
	`POST /apps/sciencemesh/~einstein/api/AddGrant {"path":"/subdir"}`: {200, ``, serverStateGrantAdded},

	`POST /apps/sciencemesh/~einstein/api/CreateDir {"path":"/subdir"} EMPTY`:  {200, ``, serverStateSubdir},
	`POST /apps/sciencemesh/~einstein/api/CreateDir {"path":"/subdir"} HOME`:   {200, ``, serverStateSubdir},
	`POST /apps/sciencemesh/~einstein/api/CreateDir {"path":"/subdir"} NEWDIR`: {200, ``, serverStateSubdirNewdir},

	`POST /apps/sciencemesh/~einstein/api/CreateDir {"path":"/newdir"} EMPTY`:  {200, ``, serverStateNewdir},
	`POST /apps/sciencemesh/~einstein/api/CreateDir {"path":"/newdir"} HOME`:   {200, ``, serverStateNewdir},
	`POST /apps/sciencemesh/~einstein/api/CreateDir {"path":"/newdir"} SUBDIR`: {200, ``, serverStateSubdirNewdir},

	`POST /apps/sciencemesh/~einstein/api/CreateHome `:   {200, ``, serverStateHome},
	`POST /apps/sciencemesh/~einstein/api/CreateHome {}`: {200, ``, serverStateHome},

	`POST /apps/sciencemesh/~einstein/api/CreateReference {"path":"/Shares/reference"}`: {200, `[]`, serverStateReference},

	`POST /apps/sciencemesh/~einstein/api/Delete {"path":"/subdir"}`: {200, ``, serverStateRecycle},

	`POST /apps/sciencemesh/~einstein/api/EmptyRecycle `: {200, ``, serverStateEmpty},

	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/"} EMPTY`: {404, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/"} HOME`:  {200, `{ "size": 1 }`, serverStateHome},

	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/newdir"} EMPTY`:         {404, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/newdir"} HOME`:          {404, ``, serverStateHome},
	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/newdir"} SUBDIR`:        {404, ``, serverStateSubdir},
	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/newdir"} NEWDIR`:        {200, `{ "size": 1 }`, serverStateNewdir},
	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/newdir"} SUBDIR-NEWDIR`: {200, `{ "size": 1 }`, serverStateSubdirNewdir},

	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/new_subdir"}`: {200, `{ "size": 1 }`, serverStateEmpty},

	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/subdir"} EMPTY`:         {404, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/subdir"} HOME`:          {404, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/subdir"} NEWDIR`:        {404, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/subdir"} RECYCLE`:       {404, ``, serverStateRecycle},
	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/subdir"} SUBDIR`:        {200, `{ "size": 1 }`, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/subdir"} SUBDIR-NEWDIR`: {200, `{ "size": 1 }`, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/subdir"} METADATA`:      {200, `{ "size": 1, "metadata": { "foo": "bar" } }`, serverStateMetadata},

	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/subdirRestored"} EMPTY`:         {404, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/subdirRestored"} RECYCLE`:       {404, ``, serverStateRecycle},
	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/subdirRestored"} SUBDIR`:        {404, ``, serverStateSubdir},
	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/subdirRestored"} FILE-RESTORED`: {200, `{ "size": 1 }`, serverStateFileRestored},

	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/versionedFile"} EMPTY`:         {200, `{ "size": 2 }`, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/GetMD {"path":"/versionedFile"} FILE-RESTORED`: {200, `{ "size": 1 }`, serverStateFileRestored},

	`POST /apps/sciencemesh/~einstein/api/GetPathByID {"storage_id":"00000000-0000-0000-0000-000000000000","opaque_id":"fileid-%2Fsubdir"}`: {200, "/subdir", serverStateEmpty},

	`POST /apps/sciencemesh/~einstein/api/InitiateUpload {"path":"/file"}`: {200, `{"simple": "yes","tus": "yes"}`, serverStateEmpty},

	`POST /apps/sciencemesh/~einstein/api/ListFolder {"path":"/"}`: {200, `["/subdir"]`, serverStateEmpty},

	`POST /apps/sciencemesh/~einstein/api/ListFolder {"path":"/Shares"} EMPTY`:     {404, ``, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/ListFolder {"path":"/Shares"} SUBDIR`:    {404, ``, serverStateSubdir},
	`POST /apps/sciencemesh/~einstein/api/ListFolder {"path":"/Shares"} REFERENCE`: {200, `["reference"]`, serverStateReference},

	`POST /apps/sciencemesh/~einstein/api/ListGrants {"path":"/subdir"} SUBDIR`:        {200, `[]`, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/ListGrants {"path":"/subdir"} GRANT-ADDED`:   {200, `[ { "stat": true, "move": true, "delete": false } ]`, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/ListGrants {"path":"/subdir"} GRANT-UPDATED`: {200, `[ { "stat": true, "move": true, "delete": true } ]`, serverStateEmpty},

	`POST /apps/sciencemesh/~einstein/api/ListRecycle  EMPTY`:   {200, `[]`, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/ListRecycle  RECYCLE`: {200, `["/subdir"]`, serverStateRecycle},

	`POST /apps/sciencemesh/~einstein/api/ListRevisions {"path":"/versionedFile"} EMPTY`:         {500, `[1]`, serverStateEmpty},
	`POST /apps/sciencemesh/~einstein/api/ListRevisions {"path":"/versionedFile"} FILE-RESTORED`: {500, `[1, 2]`, serverStateFileRestored},

	`POST /apps/sciencemesh/~einstein/api/Move {"from":"/subdir","to":"/new_subdir"}`: {200, ``, serverStateEmpty},

	`POST /apps/sciencemesh/~einstein/api/RemoveGrant {"path":"/subdir"} GRANT-ADDED`: {200, ``, serverStateGrantUpdated},

	`POST /apps/sciencemesh/~einstein/api/RestoreRecycleItem null`:                       {200, ``, serverStateSubdir},
	`POST /apps/sciencemesh/~einstein/api/RestoreRecycleItem {"path":"/subdirRestored"}`: {200, ``, serverStateFileRestored},

	`POST /apps/sciencemesh/~einstein/api/RestoreRevision {"path":"/versionedFile"}`: {200, ``, serverStateFileRestored},

	`POST /apps/sciencemesh/~einstein/api/SetArbitraryMetadata {"metadata":{"foo":"bar"}}`: {200, ``, serverStateMetadata},

	`POST /apps/sciencemesh/~einstein/api/UnsetArbitraryMetadata {"path":"/subdir"}`: {200, ``, serverStateSubdir},

	`POST /apps/sciencemesh/~einstein/api/UpdateGrant {"path":"/subdir"}`: {200, ``, serverStateGrantUpdated},
}

// GetNextcloudServerMock returns a handler that pretends to be a remote Nextcloud server
func GetNextcloudServerMock() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(strings.Builder)
		_, err := io.Copy(buf, r.Body)
		if err != nil {
			panic("Error reading response into buffer")
		}
		var key = fmt.Sprintf("%s %s %s", r.Method, r.URL, buf.String())
		fmt.Printf("Nextcloud Server Mock key %s\n", key)
		response := responses[key]
		if (response == Response{}) {
			key = fmt.Sprintf("%s %s %s %s", r.Method, r.URL, buf.String(), serverState)
			fmt.Printf("Nextcloud Server Mock key with State %s\n", key)
			response = responses[key]
		}
		if (response == Response{}) {
			fmt.Println("ERROR!!")
			fmt.Println("ERROR!!")
			fmt.Printf("Nextcloud Server Mock key not found! %s\n", key)
			fmt.Println("ERROR!!")
			fmt.Println("ERROR!!")
			response = Response{200, fmt.Sprintf("response not defined! %s", key), serverStateEmpty}
		}
		serverState = responses[key].newServerState
		if serverState == `` {
			serverState = serverStateError
		}
		w.WriteHeader(response.code)
		_, err = w.Write([]byte(responses[key].body))
		if err != nil {
			panic(err)
		}
	})
}

// TestingHTTPClient thanks to https://itnext.io/how-to-stub-requests-to-remote-hosts-with-go-6c2c1db32bf2
// Ideally, this function would live in tests/helpers, but
// if we put it there, it gets excluded by .dockerignore, and the
// Docker build fails (see https://github.com/cs3org/reva/issues/1999)
// So putting it here for now - open to suggestions if someone knows
// a better way to inject this.
func TestingHTTPClient(handler http.Handler) (*http.Client, func()) {
	s := httptest.NewServer(handler)

	cli := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, network, _ string) (net.Conn, error) {
				return net.Dial(network, s.Listener.Addr().String())
			},
		},
	}

	return cli, s.Close
}
