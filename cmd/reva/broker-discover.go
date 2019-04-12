package main

import (
	"fmt"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageregistryv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageregistry/v0alpha"
)

func brokerDiscoverCommand() *command {
	cmd := newCommand("broker-discover")
	cmd.Description = func() string {
		return "returns a list of all available storage providers known by the broker"
	}
	cmd.Action = func() error {
		req := &storageregistryv0alphapb.ListStorageProvidersRequest{}
		client, err := getStorageBrokerClient()
		if err != nil {
			return err
		}
		ctx := getAuthContext()
		res, err := client.ListStorageProviders(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		providers := res.Providers
		for _, p := range providers {
			fmt.Printf("%s => %s\n", p.ProviderPath, p.Address)
		}
		return nil
	}
	return cmd
}
