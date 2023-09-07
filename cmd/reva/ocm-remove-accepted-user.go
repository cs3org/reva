// Copyright 2018-2023 CERN
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
	"errors"
	"fmt"
	"io"

	userv1beta1 "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	rpcv1beta1 "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
)

func ocmRemoveAcceptedUser() *command {
	cmd := newCommand("ocm-remove-accepted-user")
	cmd.Description = func() string { return "remove a remote user from the personal user list" }
	cmd.Usage = func() string { return "Usage: ocm-remove-accepted-user [-flags]" }

	user := cmd.String("user", "", "the user id")
	idp := cmd.String("idp", "", "the idp of the user")

	cmd.ResetFlags = func() {
		*user, *idp = "", ""
	}

	cmd.Action = func(w ...io.Writer) error {
		// validate flags
		if *user == "" {
			return errors.New("User cannot be empty: user -user flag\n" + cmd.Usage())
		}

		if *idp == "" {
			return errors.New("IdP cannot be empty: use -idp flag\n" + cmd.Usage())
		}

		ctx := getAuthContext()
		client, err := getClient()
		if err != nil {
			return err
		}

		res, err := client.DeleteAcceptedUser(ctx, &invitepb.DeleteAcceptedUserRequest{
			RemoteUserId: &userv1beta1.UserId{
				Type:     userv1beta1.UserType_USER_TYPE_FEDERATED,
				Idp:      *idp,
				OpaqueId: *user,
			},
		})
		if err != nil {
			return err
		}
		if res.Status.Code != rpcv1beta1.Code_CODE_OK {
			return formatError(res.Status)
		}

		fmt.Println("OK")
		return nil
	}
	return cmd
}
