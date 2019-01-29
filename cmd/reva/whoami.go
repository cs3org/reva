package main

import (
	"context"
	"fmt"
	"os"

	authv0alphapb "github.com/cernbox/go-cs3apis/cs3/auth/v0alpha"
	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
)

func whoamiCommand() *command {
	cmd := newCommand("whoami")
	cmd.Description = func() string { return "tells who you are" }
	tokenFlag := cmd.String("token", "", "access token to use")

	cmd.Action = func() error {
		if cmd.NArg() != 0 {
			cmd.PrintDefaults()
			os.Exit(1)
		}
		var token string
		if *tokenFlag != "" {
			token = *tokenFlag
		} else {
			// read token from file
			t, err := readToken()
			if err != nil {
				fmt.Println("the token file cannot be readed from file ", getTokenFile())
				fmt.Println("make sure you have login before with \"reva login\"")
				return err
			}
			token = t
		}

		client, err := getAuthClient()
		if err != nil {
			return err
		}

		req := &authv0alphapb.WhoAmIRequest{AccessToken: token}

		ctx := context.Background()
		res, err := client.WhoAmI(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		me := res.User
		fmt.Printf("username: %s\ndisplay_name: %s\nmail: %s\ngroups: %v\n", me.Username, me.DisplayName, me.Mail, me.Groups)
		return nil
	}
	return cmd
}
