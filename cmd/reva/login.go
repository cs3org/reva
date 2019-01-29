package main

import (
	"bufio"
	"context"
	"fmt"
	"os"

	authv0alphapb "github.com/cernbox/go-cs3apis/cs3/auth/v0alpha"
	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
)

var loginCommand = func() *command {
	cmd := newCommand("login")
	cmd.Description = func() string { return "login into the reva server" }
	cmd.Action = func() error {
		var username, password string
		if cmd.NArg() >= 2 {
			username = cmd.Args()[0]
			password = cmd.Args()[1]
		} else {
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("username: ")
			usernameInput, err := read(reader)
			if err != nil {
				return err
			}

			fmt.Print("password: ")
			passwordInput, err := readPassword(0)
			if err != nil {
				return err
			}

			username = usernameInput
			password = passwordInput
		}

		client, err := getAuthClient()
		if err != nil {
			return err
		}

		req := &authv0alphapb.GenerateAccessTokenRequest{
			Username: username,
			Password: password,
		}

		ctx := context.Background()
		res, err := client.GenerateAccessToken(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		writeToken(res.AccessToken)
		fmt.Println("OK")
		return nil
	}
	return cmd
}
