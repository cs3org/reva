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

package meshdata

import (
	"encoding/json"
	"fmt"
)

type MeshData struct {
	Sites        []*Site
	ServiceTypes []*ServiceType
}

func (meshData *MeshData) Clear() {
	meshData.Sites = nil
	meshData.ServiceTypes = nil
}

func (meshData *MeshData) ToJSON() (string, error) {
	data, err := json.MarshalIndent(meshData, "", "\t")
	if err != nil {
		return "", fmt.Errorf("unable to marshal the mesh data: %v", err)
	}
	return string(data), nil
}

func (meshData *MeshData) FromJSON(data string) error {
	meshData.Clear()
	if err := json.Unmarshal([]byte(data), meshData); err != nil {
		return fmt.Errorf("unable to unmarshal the mesh data: %v", err)
	}
	return nil
}

func (meshData *MeshData) Clone() *MeshData {
	clone := &MeshData{}

	// To avoid any "deep copy" packages, use JSON en- and decoding instead
	data, err := meshData.ToJSON()
	if err == nil {
		if err := clone.FromJSON(data); err != nil {
			// In case of an error, clear the data
			clone.Clear()
		}
	}

	return clone
}

func (meshData *MeshData) Compare(other *MeshData) bool {
	// To avoid cumbersome comparisons, just compare the JSON-encoded data
	json1, _ := meshData.ToJSON()
	json2, _ := other.ToJSON()
	return json1 == json2
}

func New() *MeshData {
	meshData := &MeshData{}
	meshData.Clear()
	return meshData
}
