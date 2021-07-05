// Copyright 2018-2021 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package user

import (
	"context"
	"net/rpc"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/hashicorp/go-plugin"
)

// UserProviderPlugin is the implemenation of plugin.Plugin so we can serve/consume this.
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

// GetUserArg for RPC
type GetUserArg struct {
	Uid *userpb.UserId
}

// GetUserReply for RPC
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

// GetUserByClaimArg for RPC
type GetUserByClaimArg struct {
	Claim string
	Value string
}

// GetUserByClaimReply for RPC
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

// GetUserGroupsArg for RPC
type GetUserGroupsArg struct {
	User *userpb.UserId
}

// GetUserGroupsReply for RPC
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

// FindUsersArg for RPC
type FindUsersArg struct {
	Query string
}

// FindUserReply for RPC
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

// RPCServer is the server that RPCClient talks to, conforming to the requirements of net/rpc
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
