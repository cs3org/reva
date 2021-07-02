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
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Response struct {
	code           int
	body           string
	newServerState string
}

const ServerStateError = "ERROR"
const ServerStateEmpty = "EMPTY"
const ServerStateHome = "HOME"
const ServerStateSubdir = "SUBDIR"
const ServerStateNewdir = "NEWDIR"
const ServerStateSubdirNewdir = "SUBDIR-NEWDIR"
const ServerStateFileRestored = "FILE-RESTORED"
const ServerStateGrantAdded = "GRANT-ADDED"
const ServerStateGrantUpdated = "GRANT-UPDATED"
const ServerStateRecycle = "RECYCLE"
const ServerStateReference = "REFERENCE"
const ServerStateMetadata = "METADATA"

var ServerState = ServerStateEmpty

var responses = map[string]Response{
	`POST /apps/sciencemesh/~alice/AddGrant {"path":"/subdir"}`: {200, ``, ServerStateGrantAdded},

	`POST /apps/sciencemesh/~alice/CreateDir {"path":"/subdir"} EMPTY`:  {200, ``, ServerStateSubdir},
	`POST /apps/sciencemesh/~alice/CreateDir {"path":"/subdir"} HOME`:   {200, ``, ServerStateSubdir},
	`POST /apps/sciencemesh/~alice/CreateDir {"path":"/subdir"} NEWDIR`: {200, ``, ServerStateSubdirNewdir},

	`POST /apps/sciencemesh/~alice/CreateDir {"path":"/newdir"} EMPTY`:  {200, ``, ServerStateNewdir},
	`POST /apps/sciencemesh/~alice/CreateDir {"path":"/newdir"} HOME`:   {200, ``, ServerStateNewdir},
	`POST /apps/sciencemesh/~alice/CreateDir {"path":"/newdir"} SUBDIR`: {200, ``, ServerStateSubdirNewdir},

	`POST /apps/sciencemesh/~alice/CreateHome `:   {200, ``, ServerStateHome},
	`POST /apps/sciencemesh/~alice/CreateHome {}`: {200, ``, ServerStateHome},

	`POST /apps/sciencemesh/~alice/CreateReference {"path":"/Shares/reference"}`: {200, `[]`, ServerStateReference},

	`POST /apps/sciencemesh/~alice/Delete {"path":"/subdir"}`: {200, ``, ServerStateRecycle},

	`POST /apps/sciencemesh/~alice/EmptyRecycle `: {200, ``, ServerStateEmpty},

	`POST /apps/sciencemesh/~alice/GetMD {"path":"/"} EMPTY`: {404, ``, ServerStateEmpty},
	`POST /apps/sciencemesh/~alice/GetMD {"path":"/"} HOME`:  {200, `{ "size": 1 }`, ServerStateHome},

	`POST /apps/sciencemesh/~alice/GetMD {"path":"/newdir"} EMPTY`:         {404, ``, ServerStateEmpty},
	`POST /apps/sciencemesh/~alice/GetMD {"path":"/newdir"} HOME`:          {404, ``, ServerStateHome},
	`POST /apps/sciencemesh/~alice/GetMD {"path":"/newdir"} SUBDIR`:        {404, ``, ServerStateSubdir},
	`POST /apps/sciencemesh/~alice/GetMD {"path":"/newdir"} NEWDIR`:        {200, `{ "size": 1 }`, ServerStateNewdir},
	`POST /apps/sciencemesh/~alice/GetMD {"path":"/newdir"} SUBDIR-NEWDIR`: {200, `{ "size": 1 }`, ServerStateSubdirNewdir},

	`POST /apps/sciencemesh/~alice/GetMD {"path":"/new_subdir"}`: {200, `{ "size": 1 }`, ServerStateEmpty},

	`POST /apps/sciencemesh/~alice/GetMD {"path":"/subdir"} EMPTY`:         {404, ``, ServerStateEmpty},
	`POST /apps/sciencemesh/~alice/GetMD {"path":"/subdir"} HOME`:          {404, ``, ServerStateEmpty},
	`POST /apps/sciencemesh/~alice/GetMD {"path":"/subdir"} NEWDIR`:        {404, ``, ServerStateEmpty},
	`POST /apps/sciencemesh/~alice/GetMD {"path":"/subdir"} RECYCLE`:       {404, ``, ServerStateRecycle},
	`POST /apps/sciencemesh/~alice/GetMD {"path":"/subdir"} SUBDIR`:        {200, `{ "size": 1 }`, ServerStateEmpty},
	`POST /apps/sciencemesh/~alice/GetMD {"path":"/subdir"} SUBDIR-NEWDIR`: {200, `{ "size": 1 }`, ServerStateEmpty},
	`POST /apps/sciencemesh/~alice/GetMD {"path":"/subdir"} METADATA`:      {200, `{ "size": 1, "metadata": { "foo": "bar" } }`, ServerStateMetadata},

	`POST /apps/sciencemesh/~alice/GetMD {"path":"/subdirRestored"} EMPTY`:         {404, ``, ServerStateEmpty},
	`POST /apps/sciencemesh/~alice/GetMD {"path":"/subdirRestored"} RECYCLE`:       {404, ``, ServerStateRecycle},
	`POST /apps/sciencemesh/~alice/GetMD {"path":"/subdirRestored"} SUBDIR`:        {404, ``, ServerStateSubdir},
	`POST /apps/sciencemesh/~alice/GetMD {"path":"/subdirRestored"} FILE-RESTORED`: {200, `{ "size": 1 }`, ServerStateFileRestored},

	`POST /apps/sciencemesh/~alice/GetMD {"path":"/versionedFile"} EMPTY`:         {200, `{ "size": 2 }`, ServerStateEmpty},
	`POST /apps/sciencemesh/~alice/GetMD {"path":"/versionedFile"} FILE-RESTORED`: {200, `{ "size": 1 }`, ServerStateFileRestored},

	`POST /apps/sciencemesh/~alice/GetPathByID {"storage_id":"00000000-0000-0000-0000-000000000000","opaque_id":"fileid-%2Fsubdir"}`: {200, "/subdir", ServerStateEmpty},

	`POST /apps/sciencemesh/~alice/InitiateUpload {"path":"/file"}`: {200, `{"simple": "yes","tus": "yes"}`, ServerStateEmpty},

	`POST /apps/sciencemesh/~alice/ListFolder {"path":"/"}`: {200, `["/subdir"]`, ServerStateEmpty},

	`POST /apps/sciencemesh/~alice/ListFolder {"path":"/Shares"} EMPTY`:     {404, ``, ServerStateEmpty},
	`POST /apps/sciencemesh/~alice/ListFolder {"path":"/Shares"} SUBDIR`:    {404, ``, ServerStateSubdir},
	`POST /apps/sciencemesh/~alice/ListFolder {"path":"/Shares"} REFERENCE`: {200, `["reference"]`, ServerStateReference},

	`POST /apps/sciencemesh/~alice/ListGrants {"path":"/subdir"} SUBDIR`:        {200, `[]`, ServerStateEmpty},
	`POST /apps/sciencemesh/~alice/ListGrants {"path":"/subdir"} GRANT-ADDED`:   {200, `[ { "stat": true, "move": true, "delete": false } ]`, ServerStateEmpty},
	`POST /apps/sciencemesh/~alice/ListGrants {"path":"/subdir"} GRANT-UPDATED`: {200, `[ { "stat": true, "move": true, "delete": true } ]`, ServerStateEmpty},

	`POST /apps/sciencemesh/~alice/ListRecycle  EMPTY`:   {200, `[]`, ServerStateEmpty},
	`POST /apps/sciencemesh/~alice/ListRecycle  RECYCLE`: {200, `["/subdir"]`, ServerStateRecycle},

	`POST /apps/sciencemesh/~alice/ListRevisions {"path":"/versionedFile"} EMPTY`:         {500, `[1]`, ServerStateEmpty},
	`POST /apps/sciencemesh/~alice/ListRevisions {"path":"/versionedFile"} FILE-RESTORED`: {500, `[1, 2]`, ServerStateFileRestored},

	`POST /apps/sciencemesh/~alice/Move {"from":"/subdir","to":"/new_subdir"}`: {200, ``, ServerStateEmpty},

	`POST /apps/sciencemesh/~alice/RemoveGrant {"path":"/subdir"} GRANT-ADDED`: {200, ``, ServerStateGrantUpdated},

	`POST /apps/sciencemesh/~alice/RestoreRecycleItem null`:                       {200, ``, ServerStateSubdir},
	`POST /apps/sciencemesh/~alice/RestoreRecycleItem {"path":"/subdirRestored"}`: {200, ``, ServerStateFileRestored},

	`POST /apps/sciencemesh/~alice/RestoreRevision {"path":"/versionedFile"}`: {200, ``, ServerStateFileRestored},

	`POST /apps/sciencemesh/~alice/SetArbitraryMetadata {"metadata":{"foo":"bar"}}`: {200, ``, ServerStateMetadata},

	`POST /apps/sciencemesh/~alice/UnsetArbitraryMetadata {"path":"/subdir"}`: {200, ``, ServerStateSubdir},

	`POST /apps/sciencemesh/~alice/UpdateGrant {"path":"/subdir"}`: {200, ``, ServerStateGrantUpdated},
}

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
			key = fmt.Sprintf("%s %s %s %s", r.Method, r.URL, buf.String(), ServerState)
			fmt.Printf("Nextcloud Server Mock key with State %s\n", key)
			response = responses[key]
		}
		if (response == Response{}) {
			fmt.Println("ERROR!!")
			fmt.Println("ERROR!!")
			fmt.Printf("Nextcloud Server Mock key not found! %s\n", key)
			fmt.Println("ERROR!!")
			fmt.Println("ERROR!!")
			response = Response{200, fmt.Sprintf("response not defined! %s", key), ServerStateEmpty}
		}
		ServerState = responses[key].newServerState
		if ServerState == `` {
			ServerState = ServerStateError
		}
		w.WriteHeader(response.code)
		_, err = w.Write([]byte(responses[key].body))
		if err != nil {
			panic(err)
		}
	})
}
