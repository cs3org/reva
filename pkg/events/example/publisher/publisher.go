// Package publisher contains an example implementation for a publisher
package publisher

import (
	"log"

	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/events"
)

// Example publishes events to the queue
func Example(p events.Publisher) {
	// nothing to do - just publish!
	err := events.Publish(p, events.ShareCreated{
		Sharer: &user.UserId{
			OpaqueId: "123",
		},
		GranteeUserID: &user.UserId{
			OpaqueId: "456",
		},
	})
	if err != nil {
		log.Fatal(err)
	}

}
