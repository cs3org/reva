package main

import (
	"context"
	"fmt"
	"os"

	appproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/appprovider/v0alpha"
	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
)

func appProviderGetIFrameCommand() *command {
	cmd := newCommand("app-provider-get-iframe")
	cmd.Description = func() string {
		return "find iframe UI provider for filename"
	}
	cmd.Action = func() error {
		if cmd.NArg() < 3 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		appProvider := cmd.Args()[0]
		fn := cmd.Args()[1]
		token := cmd.Args()[2]
		req := &appproviderv0alphapb.GetIFrameRequest{
			Filename:    fn,
			AccessToken: token,
		}

		client, err := getAppProviderClient(appProvider)
		if err != nil {
			return err
		}
		ctx := context.Background()
		res, err := client.GetIFrame(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		fmt.Printf("Load in your browser the following iframe to edit the resource: %s", res.IframeLocation)
		return nil
	}
	return cmd
}
