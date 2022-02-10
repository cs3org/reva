package events

import (
	"encoding/json"

	group "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
)

// ShareCreated is emitted when a share is created
type ShareCreated struct { // TODO: Rename to ShareCreatedEvent?
	Sharer *user.UserId
	// split the protobuf Grantee oneof so we can use stdlib encoding/json
	GranteeUserID  *user.UserId
	GranteeGroupID *group.GroupId
	Sharee         *provider.Grantee
	ItemID         *provider.ResourceId
	CTime          *types.Timestamp
}

// Unmarshal to fulfill umarshaller interface
func (ShareCreated) Unmarshal(v []byte) (interface{}, error) {
	e := ShareCreated{}
	err := json.Unmarshal(v, &e)
	return e, err
}
