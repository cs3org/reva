package eosgrpc

import (
	"fmt"
	"strings"

	erpc "github.com/cern-eos/go-eosgrpc"
	"github.com/cs3org/reva/pkg/eosclient"
	"github.com/cs3org/reva/pkg/errtypes"
	"github.com/cs3org/reva/pkg/storage/utils/acl"
)

// If the error is not nil, take that
// If there is an error coming from EOS, return a descriptive error.
func (c *Client) getRespError(rsp *erpc.NSResponse, err error) error {
	if err != nil {
		return err
	}
	if rsp == nil || rsp.Error == nil || rsp.Error.Code == 0 {
		return nil
	}

	switch rsp.Error.Code {
	case 16: // EBUSY
		return eosclient.FileIsLockedError
	case 17: // EEXIST
		return eosclient.AttrAlreadyExistsError
	default:
		return errtypes.InternalError(fmt.Sprintf("%s (code: %d)", rsp.Error.Msg, rsp.Error.Code))
	}
}

func aclAttrToAclStruct(aclAttr string) *acl.ACLs {
	entries := strings.Split(aclAttr, ",")

	acl := &acl.ACLs{}

	for _, entry := range entries {
		parts := strings.Split(entry, ":")
		if len(parts) != 3 {
			continue
		}
		aclType := parts[0]
		qualifier := parts[1]
		permissions := parts[2]

		acl.SetEntry(aclType, qualifier, permissions)
	}

	return acl
}

func attrTypeToString(at eosclient.AttrType) string {
	switch at {
	case eosclient.SystemAttr:
		return "sys"
	case eosclient.UserAttr:
		return "user"
	default:
		return "invalid"
	}
}
