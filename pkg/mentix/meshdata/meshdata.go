// Copyright 2018-2023 CERN
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
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

const (
	// StatusDefault signals that this is just regular data.
	StatusDefault = iota

	// StatusObsolete flags the mesh data for removal.
	StatusObsolete
)

// MeshData holds the entire mesh data managed by Mentix.
type MeshData struct {
	Operators    []*Operator
	ServiceTypes []*ServiceType

	Status int `json:"-"`
}

// Clear removes all saved data, leaving an empty mesh.
func (meshData *MeshData) Clear() {
	meshData.Operators = nil
	meshData.ServiceTypes = nil

	meshData.Status = StatusDefault
}

// AddOperator adds a new operator; if an operator with the same ID already exists, the existing one is overwritten.
func (meshData *MeshData) AddOperator(op *Operator) {
	if opExisting := meshData.FindOperator(op.ID); opExisting != nil {
		*opExisting = *op
	} else {
		meshData.Operators = append(meshData.Operators, op)
	}
}

// RemoveOperator removes the provided operator.
func (meshData *MeshData) RemoveOperator(op *Operator) {
	for idx, opExisting := range meshData.Operators {
		if strings.EqualFold(opExisting.ID, op.ID) { // Remove the operator by its ID
			lastIdx := len(meshData.Operators) - 1
			meshData.Operators[idx] = meshData.Operators[lastIdx]
			meshData.Operators[lastIdx] = nil
			meshData.Operators = meshData.Operators[:lastIdx]
			break
		}
	}
}

// FindOperator searches for an operator with the given ID.
func (meshData *MeshData) FindOperator(id string) *Operator {
	for _, op := range meshData.Operators {
		if strings.EqualFold(op.ID, id) {
			return op
		}
	}
	return nil
}

// AddServiceType adds a new service type; if a type with the same name already exists, the existing one is overwritten.
func (meshData *MeshData) AddServiceType(serviceType *ServiceType) {
	if svcTypeExisting := meshData.FindServiceType(serviceType.Name); svcTypeExisting != nil {
		*svcTypeExisting = *serviceType
	} else {
		meshData.ServiceTypes = append(meshData.ServiceTypes, serviceType)
	}
}

// RemoveServiceType removes the provided service type.
func (meshData *MeshData) RemoveServiceType(serviceType *ServiceType) {
	for idx, svcTypeExisting := range meshData.ServiceTypes {
		if strings.EqualFold(svcTypeExisting.Name, serviceType.Name) { // Remove the service type by its name
			lastIdx := len(meshData.ServiceTypes) - 1
			meshData.ServiceTypes[idx] = meshData.ServiceTypes[lastIdx]
			meshData.ServiceTypes[lastIdx] = nil
			meshData.ServiceTypes = meshData.ServiceTypes[:lastIdx]
			break
		}
	}
}

// FindServiceType searches for a service type with the given name.
func (meshData *MeshData) FindServiceType(name string) *ServiceType {
	for _, serviceType := range meshData.ServiceTypes {
		if strings.EqualFold(serviceType.Name, name) {
			return serviceType
		}
	}
	return nil
}

// Merge merges data from another MeshData instance into this one.
func (meshData *MeshData) Merge(inData *MeshData) {
	for _, op := range inData.Operators {
		meshData.AddOperator(op)
	}

	for _, serviceType := range inData.ServiceTypes {
		meshData.AddServiceType(serviceType)
	}
}

// Verify checks if the mesh data is valid.
func (meshData *MeshData) Verify() error {
	// Verify all operators
	for _, op := range meshData.Operators {
		if err := op.Verify(); err != nil {
			return err
		}
	}

	// Verify all service types
	for _, serviceType := range meshData.ServiceTypes {
		if err := serviceType.Verify(); err != nil {
			return err
		}
	}

	return nil
}

// InferMissingData infers missing data from other data where possible.
func (meshData *MeshData) InferMissingData() {
	// Infer missing operator data
	for _, op := range meshData.Operators {
		op.InferMissingData()
	}

	// Infer missing service type data
	for _, serviceType := range meshData.ServiceTypes {
		serviceType.InferMissingData()
	}
}

// ToJSON converts the data to JSON.
func (meshData *MeshData) ToJSON() (string, error) {
	data, err := json.MarshalIndent(meshData, "", "\t")
	if err != nil {
		return "", fmt.Errorf("unable to marshal the mesh data: %v", err)
	}
	return string(data), nil
}

// FromJSON converts JSON data to mesh data.
func (meshData *MeshData) FromJSON(data string) error {
	meshData.Clear()
	if err := json.Unmarshal([]byte(data), meshData); err != nil {
		return fmt.Errorf("unable to unmarshal the mesh data: %v", err)
	}
	return nil
}

// Clone creates an exact copy of the mesh data.
func (meshData *MeshData) Clone() *MeshData {
	clone := &MeshData{}

	// To avoid any "deep copy" packages, use gob en- and decoding instead
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	dec := gob.NewDecoder(&buf)

	if err := enc.Encode(meshData); err == nil {
		if err := dec.Decode(clone); err != nil {
			// In case of an error, clear the data
			clone.Clear()
		}
	}

	return clone
}

// Compare checks whether the stored data equals the data of another MeshData object.
func (meshData *MeshData) Compare(other *MeshData) bool {
	return reflect.DeepEqual(meshData, other)
}

// New returns a new (empty) MeshData object.
func New() *MeshData {
	meshData := &MeshData{}
	meshData.Clear()
	return meshData
}
