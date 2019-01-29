package impersonator

import (
	"context"
	"testing"
)

func TestImpersonator(t *testing.T) {
	ctx := context.Background()
	i := New()
	i.Authenticate(ctx, "admin", "pwd")
}
