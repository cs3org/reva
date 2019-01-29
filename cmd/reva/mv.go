package main

import (
	"context"
	"fmt"
	"os"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
)

func moveCommand() *command {
	cmd := newCommand("mv")
	cmd.Description = func() string { return "moves/rename a file/folder" }
	cmd.Action = func() error {
		if cmd.NArg() < 3 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		provider := cmd.Args()[0]
		src := cmd.Args()[1]
		dst := cmd.Args()[2]

		ctx := context.Background()
		client, err := getStorageProviderClient(provider)
		if err != nil {
			return err
		}

		req := &storageproviderv0alphapb.MoveRequest{SourceFilename: src, TargetFilename: dst}
		res, err := client.Move(ctx, req)
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
