package eventsmiddleware

import (
	collaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	"github.com/cs3org/reva/pkg/events"
)

// ShareCreated converts response to event
func ShareCreated(r *collaboration.CreateShareResponse) events.ShareCreated {
	return events.ShareCreated{
		Sharer: r.Share.Creator,
		//Sharee: r.Share.Grantee, // TODO: unmarshaling fails -> find out why
		ItemID: r.Share.ResourceId,
		CTime:  r.Share.Ctime,
	}
}
