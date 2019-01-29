package main

import (
	"context"
	"fmt"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storagebrokerv0alphapb "github.com/cernbox/go-cs3apis/cs3/storagebroker/v0alpha"
)

func brokerFindCommand() *command {
	cmd := newCommand("broker-find")
	cmd.Description = func() string {
		return "find storage provider for path"
	}
	cmd.Action = func() error {
		fn := "/"
		if cmd.NArg() >= 1 {
			fn = cmd.Args()[0]
		}

		req := &storagebrokerv0alphapb.FindRequest{
			Filename: fn,
		}
		client, err := getStorageBrokerClient()
		if err != nil {
			return err
		}
		ctx := context.Background()
		res, err := client.Find(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		fmt.Printf("resource can be found at %s\n", res.ProviderInfo.Location)
		return nil
	}
	return cmd
}
