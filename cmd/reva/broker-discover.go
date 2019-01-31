package main

import (
	"context"
	"fmt"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storagebrokerv0alphapb "github.com/cernbox/go-cs3apis/cs3/storagebroker/v0alpha"
)

func brokerDiscoverCommand() *command {
	cmd := newCommand("broker-discover")
	cmd.Description = func() string {
		return "returns a list of all available storage providers known by the broker"
	}
	cmd.Action = func() error {
		req := &storagebrokerv0alphapb.DiscoverRequest{}
		client, err := getStorageBrokerClient()
		if err != nil {
			return err
		}
		ctx := context.Background()
		res, err := client.Discover(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		providers := res.StorageProviders
		for _, p := range providers {
			fmt.Printf("%s => %s\n", p.MountPath, p.Endpoint)
		}
		return nil
	}
	return cmd
}
