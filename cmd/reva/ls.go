package main

import (
	"context"
	"fmt"
	"io"
	"os"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
)

func lsCommand() *command {
	cmd := newCommand("ls")
	cmd.Description = func() string { return "list a folder contents" }
	longFlag := cmd.Bool("l", false, "long listing")
	cmd.Action = func() error {
		if cmd.NArg() < 2 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		provider := cmd.Args()[0]
		fn := cmd.Args()[1]
		client, err := getStorageProviderClient(provider)
		if err != nil {
			return err
		}

		req := &storageproviderv0alphapb.ListRequest{
			Filename: fn,
		}

		ctx := context.Background()
		stream, err := client.List(ctx, req)
		if err != nil {
			return err
		}

		mds := []*storageproviderv0alphapb.Metadata{}
		for {
			res, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if res.Status.Code != rpcpb.Code_CODE_OK {
				return formatError(res.Status)
			}
			mds = append(mds, res.Metadata)
		}

		for _, md := range mds {
			if *longFlag {
				fmt.Printf("%+v %d %d %s\n", md.Permissions, md.Mtime, md.Size, md.Filename)
			} else {
				fmt.Println(md.Filename)
			}
		}
		return nil
	}
	return cmd
}
