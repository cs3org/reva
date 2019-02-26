package main

import (
	"fmt"
	"os"

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

		ref := &storageproviderv0alphapb.Reference{
			Spec: &storageproviderv0alphapb.Reference_Path{Path: fn},
		}
		req := &storageproviderv0alphapb.ListContainerRequest{Ref: ref}

		ctx := getAuthContext()
		res, err := client.ListContainer(ctx, req)
		if err != nil {
			return err
		}

		infos := res.Infos
		for _, info := range infos {
			if *longFlag {
				fmt.Printf("%+v %d %d %v %s\n", info.PermissionSet, info.Mtime, info.Size, info.Id, info.Path)
			} else {
				fmt.Println(info.Path)
			}
		}
		return nil
	}
	return cmd
}
