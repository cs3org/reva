// Copyright 2018-2022 CERN
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

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	gateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	user "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	types "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
)

func setlockCommand() *command {
	cmd := newCommand("setlock")
	cmd.Description = func() string { return "set a lock on a resource" }
	cmd.Usage = func() string { return "Usage: setlock [-flags] <resource_path>" }

	typeFlag := cmd.String("type", "write", "type of lock")
	idFlag := cmd.String("id", "", "id of lock")
	userFlag := cmd.String("user", "", "user associated to lock")
	appFlag := cmd.String("app", "", "app associated to lock")
	expFlag := cmd.String("exp", "", "lock expiration time")
	refreshFlag := cmd.Bool("refresh", false, "refresh the lock")

	cmd.ResetFlags = func() {
		*typeFlag, *idFlag, *userFlag, *appFlag, *expFlag, *refreshFlag = "", "", "", "", "", false
	}

	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 1 {
			return errors.New("Invalid arguments: " + cmd.Usage())
		}

		fn := cmd.Args()[0]
		client, err := getClient()
		if err != nil {
			return err
		}

		ctx := getAuthContext()

		lock, err := createLock(ctx, client, *typeFlag, *idFlag, *userFlag, *appFlag, *expFlag)
		if err != nil {
			return err
		}

		ref := &provider.Reference{Path: fn}

		if *refreshFlag {
			res, err := client.RefreshLock(ctx, &provider.RefreshLockRequest{
				Ref:  ref,
				Lock: lock,
			})
			if err != nil {
				return err
			}

			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
		} else {
			res, err := client.SetLock(ctx, &provider.SetLockRequest{
				Ref:  ref,
				Lock: lock,
			})
			if err != nil {
				return err
			}

			if res.Status.Code != rpc.Code_CODE_OK {
				return formatError(res.Status)
			}
		}

		fmt.Println("OK")

		return nil
	}
	return cmd
}

func createLock(ctx context.Context, client gateway.GatewayAPIClient, t, id, u, app, exp string) (*provider.Lock, error) {
	lockType, err := getType(t)
	if err != nil {
		return nil, err
	}
	var uID *user.UserId
	if u != "" {
		u, err := getUser(ctx, client, u)
		if err != nil {
			return nil, err
		}
		uID = u.GetId()
	}
	var expiration *types.Timestamp
	if exp != "" {
		expiration, err = getExpiration(exp)
		if err != nil {
			return nil, err
		}
	}

	lock := provider.Lock{
		LockId:     id,
		Type:       lockType,
		User:       uID,
		AppName:    app,
		Expiration: expiration,
	}

	return &lock, nil
}

func getType(t string) (provider.LockType, error) {
	switch t {
	case "shared":
		return provider.LockType_LOCK_TYPE_SHARED, nil
	case "write":
		return provider.LockType_LOCK_TYPE_WRITE, nil
	case "exclusive":
		return provider.LockType_LOCK_TYPE_EXCL, nil
	default:
		return provider.LockType_LOCK_TYPE_INVALID, errors.New("type not recognised")
	}
}

func getUser(ctx context.Context, client gateway.GatewayAPIClient, u string) (*user.User, error) {
	res, err := client.GetUserByClaim(ctx, &user.GetUserByClaimRequest{
		Claim: "username",
		Value: u,
	})
	switch {
	case err != nil:
		return nil, err
	case res.Status.Code != rpc.Code_CODE_OK:
		return nil, errors.New(res.Status.Message)
	}
	return res.User, nil
}

func getExpiration(exp string) (*types.Timestamp, error) {
	t, err := time.Parse("2006-01-02", exp)
	if err != nil {
		return nil, err
	}
	return &types.Timestamp{
		Seconds: uint64(t.Unix()),
	}, nil
}
