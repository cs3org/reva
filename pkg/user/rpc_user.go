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
	"github.com/cs3org/reva/pkg/plugin"
	hcplugin "github.com/hashicorp/go-plugin"
)

func init() {
	plugin.Register("userprovider", &ProviderPlugin{})
}

// ProviderPlugin is the implementation of plugin.Plugin so we can serve/consume this.
type ProviderPlugin struct {
	Impl Manager
}

// Server returns the RPC Server which serves the methods that the Client calls over net/rpc
func (p *ProviderPlugin) Server(*hcplugin.MuxBroker) (interface{}, error) {
	return &RPCServer{Impl: p.Impl}, nil
}

// Client returns interface implementation for the plugin that communicates to the server end of the plugin
func (p *ProviderPlugin) Client(b *hcplugin.MuxBroker, c *rpc.Client) (interface{}, error) {
	return &RPCClient{Client: c}, nil
}

// RPCClient is an implementation of Manager that talks over RPC.
type RPCClient struct{ Client *rpc.Client }

// ConfigureArg for RPC
type ConfigureArg struct {
	Ml map[string]interface{}
}

// ConfigureReply for RPC
type ConfigureReply struct {
	Err error
}

// Configure RPCClient configure method
func (m *RPCClient) Configure(ml map[string]interface{}) error {
	args := ConfigureArg{Ml: ml}
	resp := ConfigureReply{}
	err := m.Client.Call("Plugin.Configure", args, &resp)
	if err != nil {
		return err
	}
	return resp.Err
}

// GetUserArg for RPC
type GetUserArg struct {
	Ctx *plugin.Ctx
	UID *userpb.UserId
}

// GetUserReply for RPC
type GetUserReply struct {
	User *userpb.User
	Err  error
}

// GetUser RPCClient GetUser method
func (m *RPCClient) GetUser(ctx context.Context, uid *userpb.UserId) (*userpb.User, error) {
	ctxVal, err := plugin.GetContextStruct(ctx)
	if err != nil {
		return nil, err
	}
	args := GetUserArg{Ctx: ctxVal, UID: uid}
	resp := GetUserReply{}
	err = m.Client.Call("Plugin.GetUser", args, &resp)
	if err != nil {
		return nil, err
	}
	return resp.User, resp.Err
}

// GetUserByClaimArg for RPC
type GetUserByClaimArg struct {
	Ctx   *plugin.Ctx
	Claim string
	Value string
}

// GetUserByClaimReply for RPC
type GetUserByClaimReply struct {
	User *userpb.User
	Err  error
}

// GetUserByClaim RPCClient GetUserByClaim method
func (m *RPCClient) GetUserByClaim(ctx context.Context, claim, value string) (*userpb.User, error) {
	ctxVal, err := plugin.GetContextStruct(ctx)
	if err != nil {
		return nil, err
	}
	args := GetUserByClaimArg{Ctx: ctxVal, Claim: claim, Value: value}
	resp := GetUserByClaimReply{}
	err = m.Client.Call("Plugin.GetUserByClaim", args, &resp)
	if err != nil {
		return nil, err
	}
	return resp.User, resp.Err
}

// GetUserGroupsArg for RPC
type GetUserGroupsArg struct {
	Ctx  *plugin.Ctx
	User *userpb.UserId
}

// GetUserGroupsReply for RPC
type GetUserGroupsReply struct {
	Group []string
	Err   error
}

// GetUserGroups RPCClient GetUserGroups method
func (m *RPCClient) GetUserGroups(ctx context.Context, user *userpb.UserId) ([]string, error) {
	ctxVal, err := plugin.GetContextStruct(ctx)
	if err != nil {
		return nil, err
	}
	args := GetUserGroupsArg{Ctx: ctxVal, User: user}
	resp := GetUserGroupsReply{}
	err = m.Client.Call("Plugin.GetUserGroups", args, &resp)
	if err != nil {
		return nil, err
	}
	return resp.Group, resp.Err
}

// FindUsersArg for RPC
type FindUsersArg struct {
	Ctx   *plugin.Ctx
	Query string
}

// FindUsersReply for RPC
type FindUsersReply struct {
	User []*userpb.User
	Err  error
}

// FindUsers RPCClient FindUsers method
func (m *RPCClient) FindUsers(ctx context.Context, query string) ([]*userpb.User, error) {
	ctxVal, err := plugin.GetContextStruct(ctx)
	if err != nil {
		return nil, err
	}
	args := FindUsersArg{Ctx: ctxVal, Query: query}
	resp := FindUsersReply{}
	err = m.Client.Call("Plugin.FindUsers", args, &resp)
	if err != nil {
		return nil, err
	}
	return resp.User, resp.Err
}

// RPCServer is the server that RPCClient talks to, conforming to the requirements of net/rpc
type RPCServer struct {
	// This is the real implementation
	Impl Manager
}

// Configure RPCServer Configure method
func (m *RPCServer) Configure(args ConfigureArg, resp *ConfigureReply) error {
	resp.Err = m.Impl.Configure(args.Ml)
	return nil
}

// GetUser RPCServer GetUser method
func (m *RPCServer) GetUser(args GetUserArg, resp *GetUserReply) error {
	ctx := plugin.SetContext(args.Ctx)
	resp.User, resp.Err = m.Impl.GetUser(ctx, args.UID)
	return nil
}

// GetUserByClaim RPCServer GetUserByClaim method
func (m *RPCServer) GetUserByClaim(args GetUserByClaimArg, resp *GetUserByClaimReply) error {
	ctx := plugin.SetContext(args.Ctx)
	resp.User, resp.Err = m.Impl.GetUserByClaim(ctx, args.Claim, args.Value)
	return nil
}

// GetUserGroups RPCServer GetUserGroups method
func (m *RPCServer) GetUserGroups(args GetUserGroupsArg, resp *GetUserGroupsReply) error {
	ctx := plugin.SetContext(args.Ctx)
	resp.Group, resp.Err = m.Impl.GetUserGroups(ctx, args.User)
	return nil
}

// FindUsers RPCServer FindUsers method
func (m *RPCServer) FindUsers(args FindUsersArg, resp *FindUsersReply) error {
	ctx := plugin.SetContext(args.Ctx)
	resp.User, resp.Err = m.Impl.FindUsers(ctx, args.Query)
	return nil
}
