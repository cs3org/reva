package main

import (
	"errors"
	"fmt"
	"io"

	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
)

func ocmInviteForwardCommand() *command {
	cmd := newCommand("ocm-invite-forward")
	cmd.Description = func() string { return "forward ocm invite token" }
	cmd.Usage = func() string { return "Usage: ocm-invite-forward [-flags] <path>" }
	token := cmd.String("token", "", "invite token")
	idp := cmd.String("idp", "", "the idp of the user who generated the token")

	cmd.ResetFlags = func() {
		*token, *idp = "", ""
	}

	cmd.Action = func(w ...io.Writer) error {
		// validate flags
		if *token == "" {
			return errors.New("token cannot be empty: use -token flag\n" + cmd.Usage())
		}
		if *idp == "" {
			return errors.New("Provider domain cannot be empty: use -provider flag\n" + cmd.Usage())
		}

		ctx := getAuthContext()
		client, err := getClient()
		if err != nil {
			return err
		}

		inviteToken := &invitepb.InviteToken{
			Token: *token,
		}

		providerInfo, err := client.GetInfoByDomain(ctx, &ocmprovider.GetInfoByDomainRequest{
			Domain: *idp,
		})
		if err != nil {
			return err
		}

		forwardToken, err := client.ForwardInvite(ctx, &invitepb.ForwardInviteRequest{
			InviteToken:          inviteToken,
			OriginSystemProvider: providerInfo.ProviderInfo,
		})
		if err != nil {
			return err
		}

		if forwardToken.Status.Code != rpc.Code_CODE_OK {
			return formatError(forwardToken.Status)
		}
		fmt.Println(forwardToken.Status.Code)
		return nil
	}
	return cmd
}
