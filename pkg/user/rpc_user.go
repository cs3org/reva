package user

import (
	"context"
	"net/rpc"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/hashicorp/go-plugin"
)

type UserProviderPlugin struct {
	Impl UserManager
}

func (p *UserProviderPlugin) Server(*plugin.MuxBroker) (interface{}, error) {
	return &RPCServer{Impl: p.Impl}, nil
}

func (p *UserProviderPlugin) Client(b *plugin.MuxBroker, c *rpc.Client) (interface{}, error) {
	return &RPCClient{Client: c}, nil
}

// RPCClient is an implementation of Manager that talks over RPC.
type RPCClient struct{ Client *rpc.Client }

// NewArg for RPC
type NewArg struct {
	Ml map[string]interface{}
}

// NewReply for RPC
type NewReply struct {
	Err error
}

func (m *RPCClient) New(ml map[string]interface{}) error {
	args := NewArg{Ml: ml}
	resp := NewReply{}
	err := m.Client.Call("Plugin.New", args, &resp)
	if err != nil {
		return err
	}
	return resp.Err
}

type GetUserArg struct {
	Uid *userpb.UserId
}

type GetUserReply struct {
	User *userpb.User
	Err  error
}

func (m *RPCClient) GetUser(ctx context.Context, uid *userpb.UserId) (*userpb.User, error) {
	args := GetUserArg{Uid: uid}
	resp := GetUserReply{}
	err := m.Client.Call("Plugin.GetUser", args, &resp)
	if err != nil {
		return nil, err
	}
	return resp.User, resp.Err
}

type GetUserByClaimArg struct {
	Claim string
	Value string
}

type GetUserByClaimReply struct {
	User *userpb.User
	Err  error
}

func (m *RPCClient) GetUserByClaim(ctx context.Context, claim, value string) (*userpb.User, error) {
	args := GetUserByClaimArg{Claim: claim, Value: value}
	resp := GetUserByClaimReply{}
	err := m.Client.Call("Plugin.GetUserByClaim", args, &resp)
	if err != nil {
		return nil, err
	}
	return resp.User, resp.Err
}

type GetUserGroupsArg struct {
	User *userpb.UserId
}

type GetUserGroupsReply struct {
	Group []string
	Err   error
}

func (m *RPCClient) GetUserGroups(ctx context.Context, user *userpb.UserId) ([]string, error) {
	args := GetUserGroupsArg{User: user}
	resp := GetUserGroupsReply{}
	err := m.Client.Call("Plugin.GetUserGroups", args, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Group, resp.Err
}

type FindUsersArg struct {
	Query string
}

type FindUsersReply struct {
	User []*userpb.User
	Err  error
}

func (m *RPCClient) FindUsers(ctx context.Context, query string) ([]*userpb.User, error) {
	args := FindUsersArg{Query: query}
	resp := FindUsersReply{}
	err := m.Client.Call("Plugin.FindUsers", args, &resp)
	if err != nil {
		return nil, err
	}
	return resp.User, resp.Err
}

// Here is the RPC server that RPCClient talks to, conforming to
// the requirements of net/rpc
type RPCServer struct {
	// This is the real implementation
	Impl UserManager
}

func (m *RPCServer) New(args NewArg, resp *NewReply) error {
	resp.Err = m.Impl.New(args.Ml)
	return nil
}

func (m *RPCServer) GetUser(args GetUserArg, resp *GetUserReply) error {
	resp.User, resp.Err = m.Impl.GetUser(context.Background(), args.Uid)
	return nil
}

func (m *RPCServer) GetUserByClaim(args GetUserByClaimArg, resp *GetUserByClaimReply) error {
	resp.User, resp.Err = m.Impl.GetUserByClaim(context.Background(), args.Claim, args.Value)
	return nil
}

func (m *RPCServer) GetUserGroups(args GetUserGroupsArg, resp *GetUserGroupsReply) error {
	resp.Group, resp.Err = m.Impl.GetUserGroups(context.Background(), args.User)
	return nil
}

func (m *RPCServer) FindUsers(args FindUsersArg, resp *FindUsersReply) error {
	resp.User, resp.Err = m.Impl.FindUsers(context.Background(), args.Query)
	return nil
}
