package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
	"github.com/cheggaaa/pb"
)

func downloadCommand() *command {
	cmd := newCommand("download")
	cmd.Description = func() string { return "download a remote file into the local filesystem" }
	cmd.Action = func() error {
		fn := "/"
		if cmd.NArg() < 3 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		provider := cmd.Args()[0]
		fn = cmd.Args()[1]
		target := cmd.Args()[2]

		client, err := getStorageProviderClient(provider)
		if err != nil {
			return err
		}

		req1 := &storageproviderv0alphapb.StatRequest{Filename: fn}
		ctx := context.Background()
		res1, err := client.Stat(ctx, req1)
		if err != nil {
			return err
		}
		if res1.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res1.Status)
		}

		md := res1.Metadata

		fd, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}

		req2 := &storageproviderv0alphapb.ReadRequest{Filename: fn}
		ctx = context.Background()
		stream, err := client.Read(ctx, req2)
		if err != nil {
			return err
		}

		bar := pb.New(int(md.Size)).SetUnits(pb.U_BYTES)
		bar.Start()
		var reader io.Reader
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
			dc := res.DataChunk

			if dc != nil {
				if dc.Length > 0 {
					reader = bytes.NewReader(dc.Data)
					reader = bar.NewProxyReader(reader)

					_, err := io.CopyN(fd, reader, int64(dc.Length))
					if err != nil {
						return err
					}
				}
			}
		}
		bar.Finish()
		return nil

	}
	return cmd
}
