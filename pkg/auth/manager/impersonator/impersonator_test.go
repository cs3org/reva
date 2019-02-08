package impersonator

import (
	"context"
	"testing"
)

func TestImpersonator(t *testing.T) {
	ctx := context.Background()
	i, _ := New(nil)
	i.Authenticate(ctx, "admin", "pwd")
}
