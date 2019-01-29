package main

import (
	"context"
	"fmt"
	"os"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
)

func mkdirCommand() *command {
	cmd := newCommand("mkdir")
	cmd.Description = func() string { return "creates a folder" }
	cmd.Action = func() error {
		if cmd.NArg() < 2 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		fn := cmd.Args()[0]
		provider := cmd.Args()[1]

		ctx := context.Background()
		client, err := getStorageProviderClient(provider)
		if err != nil {
			return err
		}

		req := &storageproviderv0alphapb.CreateDirectoryRequest{Filename: fn}
		res, err := client.CreateDirectory(ctx, req)
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
