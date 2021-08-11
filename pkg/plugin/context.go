package plugin

import (
	"context"
	"fmt"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	ctxpkg "github.com/cs3org/reva/pkg/ctx"
)

// Ctx represents context to be passed to the plugins
type Ctx struct {
	User  *userpb.User
	Token string
}

// GetContextStruct retrieves context KV pairs and stores it into Ctx
func GetContextStruct(ctx context.Context) (*Ctx, error) {
	var ok bool
	ctxVal := &Ctx{}
	ctxVal.User, ok = ctxpkg.ContextGetUser(ctx)
	if !ok {
		return nil, fmt.Errorf("cannot get user context")
	}
	ctxVal.Token, ok = ctxpkg.ContextGetToken(ctx)
	if !ok {
		return nil, fmt.Errorf("cannot get token context")
	}
	return ctxVal, nil
}

// SetContext sets the context
func SetContext(ctxStruct *Ctx) context.Context {
	ctx := context.Background()
	ctx = ctxpkg.ContextSetUser(ctx, ctxStruct.User)
	ctx = ctxpkg.ContextSetToken(ctx, ctxStruct.Token)
	return ctx
}
