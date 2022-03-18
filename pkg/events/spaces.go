package events

import (
	"encoding/json"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
)

// SpaceCreated is emitted when a space is created
type SpaceCreated struct {
	ID    *provider.StorageSpaceId
	Owner *user.UserId
	Root  *provider.ResourceId
	Name  string
	Type  string
	Quota *provider.Quota
	MTime *types.Timestamp
}

// Unmarshal to fulfill umarshaller interface
func (SpaceCreated) Unmarshal(v []byte) (interface{}, error) {
	e := SpaceCreated{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// SpaceRenamed is emitted when a space is renamed
type SpaceRenamed struct {
	ID    *provider.StorageSpaceId
	Owner *user.UserId
	Name  string
}

// Unmarshal to fulfill umarshaller interface
func (SpaceRenamed) Unmarshal(v []byte) (interface{}, error) {
	e := SpaceRenamed{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// SpaceDisabled is emitted when a space is disabled
type SpaceDisabled struct {
	ID *provider.StorageSpaceId
}

// Unmarshal to fulfill umarshaller interface
func (SpaceDisabled) Unmarshal(v []byte) (interface{}, error) {
	e := SpaceDisabled{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// SpaceEnabled is emitted when a space is (re-)enabled
type SpaceEnabled struct {
	ID    *provider.StorageSpaceId
	Owner *user.UserId
}

// Unmarshal to fulfill umarshaller interface
func (SpaceEnabled) Unmarshal(v []byte) (interface{}, error) {
	e := SpaceEnabled{}
	err := json.Unmarshal(v, &e)
	return e, err
}

// SpaceDeleted is emitted when a space is deleted
type SpaceDeleted struct {
	ID *provider.StorageSpaceId
}

// Unmarshal to fulfill umarshaller interface
func (SpaceDeleted) Unmarshal(v []byte) (interface{}, error) {
	e := SpaceDeleted{}
	err := json.Unmarshal(v, &e)
	return e, err
}
