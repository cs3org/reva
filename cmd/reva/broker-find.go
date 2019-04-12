package main

import (
	"fmt"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
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

		req := &storageregistryv0alphapb.GetStorageProviderRequest{
			Ref: &storageproviderv0alphapb.Reference{Spec: &storageproviderv0alphapb.Reference_Path{Path: fn}},
		}
		client, err := getStorageBrokerClient()
		if err != nil {
			return err
		}
		ctx := getAuthContext()
		res, err := client.GetStorageProvider(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		fmt.Printf("resource can be found at %s\n", res.Provider.Address)
		return nil
	}
	return cmd
}
