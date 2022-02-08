package events

import (
	"encoding/json"
	"fmt"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

// UnmarshalEvent unmarshals an event, returning the correct type
func UnmarshalEvent(typ string, b []byte) (interface{}, error) {
	switch typ {
	case "events.ShareCreated":
		e := ShareCreated{}
		err := json.Unmarshal(b, &e)
		return e, err
	default:
		return nil, fmt.Errorf("unknown event: %s", typ)
	}
}

// ShareCreated is emitted when a share is created
type ShareCreated struct { // TODO: Rename to ShareCreatedEvent?
	SharerID string
	Sharee   string
	ItemID   *provider.Reference
}
