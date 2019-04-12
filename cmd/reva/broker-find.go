package main

import (
	"fmt"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageregistryv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageregistry/v0alpha"
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

		req := &storageregistryv0alphapb.FindRequest{
			Filename: fn,
		}
		client, err := getStorageBrokerClient()
		if err != nil {
			return err
		}
		ctx := getAuthContext()
		res, err := client.Find(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		fmt.Printf("resource can be found at %s\n", res.StorageProvider.Endpoint)
		return nil
	}
	return cmd
}
