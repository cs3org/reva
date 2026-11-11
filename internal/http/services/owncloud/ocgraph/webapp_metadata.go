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

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	ocmconversions "github.com/cs3org/reva/v3/pkg/ocm/conversions"
	libregraph "github.com/owncloud/libre-graph-api-go"
)

const receivedWebappJSONKey = "@ocm.webApp"

type receivedWebappMetadata = ocmconversions.ReceivedWebappMetadata

type receivedShareDriveItem struct {
	*libregraph.DriveItem
	webapp *receivedWebappMetadata
}

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
	return extendReceivedShareRemoteItem(base, d.webapp.AppName, d.webapp.Apps)
}

// extendReceivedShareRemoteItem adds @ocm.webApp as a sibling inside remoteItem.
// Every other field is kept as the raw JSON the DriveItem serializer produced.
// appName is the default (first) app, kept for clients that predate apps.
func extendReceivedShareRemoteItem(base []byte, appName string, apps []*ocm.WebappApp) ([]byte, error) {
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

	type app struct {
		Name       string   `json:"name"`
		MediaTypes []string `json:"mediaTypes"`
	}
	ext := struct {
		AppName string `json:"appName"`
		Apps    []app  `json:"apps,omitempty"`
	}{AppName: appName}
	for _, a := range apps {
		ext.Apps = append(ext.Apps, app{Name: a.GetName(), MediaTypes: a.GetMediaTypes()})
	}
	extRaw, err := json.Marshal(ext)
	if err != nil {
		return nil, err
	}
	remote[receivedWebappJSONKey] = extRaw

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
