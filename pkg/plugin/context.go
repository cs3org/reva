package plugin

import (
	"context"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
)

// Ctx represents context to be passed to the plugins
type Ctx struct {
	User  []*userpb.User
	Token []string
}

// GetContextKV retrieves context KV pairs and stores it into Ctx
func GetContextKV(ctx context.Context) *Ctx {
	ctxVal := &Ctx{}
	m := appctx.GetKeyValuesFromCtx(ctx)
	for _, v := range m {
		switch c := v.(type) {
		case string:
			ctxVal.Token = append(ctxVal.Token, c)
		case *userpb.User:
			ctxVal.User = append(ctxVal.User, c)
		}
	}
	return ctxVal
}
