package main

import (
	"context"
	"fmt"
	"os"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
)

func rmCommand() *command {
	cmd := newCommand("rm")
	cmd.Description = func() string { return "removes a file or folder" }
	cmd.Action = func() error {
		if cmd.NArg() < 2 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		provider := cmd.Args()[0]
		fn := cmd.Args()[1]
		ctx := context.Background()
		client, err := getStorageProviderClient(provider)
		if err != nil {
			return err
		}

		req := &storageproviderv0alphapb.DeleteRequest{Filename: fn}
		res, err := client.Delete(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		return nil
	}
	return cmd
}
