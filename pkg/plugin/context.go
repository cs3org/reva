package plugin

import (
	"context"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
)

type Ctx struct {
	User  []*userpb.User
	Token []string
}

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
