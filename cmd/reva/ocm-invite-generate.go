package main

import (
	"fmt"
	"io"

	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
)

func ocmInviteGenerateCommand() *command {
	cmd := newCommand("ocm-invite-generate")
	cmd.Description = func() string { return "generate ocm invitation token" }
	cmd.Usage = func() string { return "Usage: ocm-invite-generate" }

	cmd.Action = func(w ...io.Writer) error {
		ctx := getAuthContext()
		client, err := getClient()
		if err != nil {
			return err
		}

		inviteToken, err := client.GenerateInviteToken(ctx, &invitepb.GenerateInviteTokenRequest{})
		if err != nil {
			return err
		}
		if inviteToken.Status.Code != rpc.Code_CODE_OK {
			return formatError(inviteToken.Status)
		}
		fmt.Println(inviteToken)
		return nil
	}
	return cmd
}
