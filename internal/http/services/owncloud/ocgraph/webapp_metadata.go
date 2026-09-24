// Copyright 2018-2024 CERN
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

package ocgraph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	libregraph "github.com/owncloud/libre-graph-api-go"
)

const receivedWebappJSONKey = "@ocm.webApp"

type receivedWebappMetadata struct {
	Present bool
	AppName string
}

type receivedShareDriveItem struct {
	*libregraph.DriveItem
	webapp *receivedWebappMetadata
}

// encodeSharedWithMe writes the getSharedWithMe collection envelope.
func encodeSharedWithMe(w io.Writer, shares []any) error {
	return json.NewEncoder(w).Encode(map[string]any{
		"value": shares,
	})
}

func newReceivedShareDriveItem(item *libregraph.DriveItem, webapp *receivedWebappMetadata) receivedShareDriveItem {
	return receivedShareDriveItem{
		DriveItem: item,
		webapp:    webapp,
	}
}

func (d receivedShareDriveItem) MarshalJSON() ([]byte, error) {
	if d.webapp == nil || !d.webapp.Present {
		return json.Marshal(d.DriveItem)
	}
	if d.DriveItem == nil {
		return nil, fmt.Errorf("received share drive item is nil")
	}

	base, err := json.Marshal(d.DriveItem)
	if err != nil {
		return nil, err
	}
	return extendReceivedShareRemoteItem(base, d.webapp.AppName)
}

// extendReceivedShareRemoteItem adds @ocm.webApp as a sibling inside remoteItem.
// Every other field is kept as the raw JSON the DriveItem serializer produced.
func extendReceivedShareRemoteItem(base []byte, appName string) ([]byte, error) {
	root, err := jsonObject(base)
	if err != nil {
		return nil, fmt.Errorf("received share drive item JSON: %w", err)
	}

	rawRemote, ok := root["remoteItem"]
	if !ok {
		return nil, fmt.Errorf("received share drive item remoteItem is missing")
	}
	remote, err := jsonObject(rawRemote)
	if err != nil {
		return nil, fmt.Errorf("received share drive item remoteItem: %w", err)
	}

	ext, err := json.Marshal(map[string]string{
		"appName": appName,
	})
	if err != nil {
		return nil, err
	}
	remote[receivedWebappJSONKey] = ext

	remoteBytes, err := json.Marshal(remote)
	if err != nil {
		return nil, err
	}
	root["remoteItem"] = remoteBytes

	out, err := json.Marshal(root)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func jsonObject(raw []byte) (map[string]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, fmt.Errorf("json value is not an object")
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, fmt.Errorf("json value is not an object")
	}
	return obj, nil
}
