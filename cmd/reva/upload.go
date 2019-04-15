package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
)

func uploadCommand() *command {
	cmd := newCommand("upload")
	cmd.Description = func() string { return "upload a local file to the remote server" }
	cmd.Action = func() error {
		if cmd.NArg() < 3 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		provider := cmd.Args()[0]
		fn := cmd.Args()[1]
		target := cmd.Args()[2]

		fd, err := os.Open(fn)
		if err != nil {
			return err
		}
		md, err := fd.Stat()
		if err != nil {
			return err
		}
		defer fd.Close()

		fmt.Printf("Going to upload %d bytes\n", md.Size())

		ctx := getAuthContext()
		client, err := getStorageProviderClient(provider)
		if err != nil {
			return err
		}

		req := &storageproviderv0alphapb.InitiateFileUploadRequest{
			Ref: &storageproviderv0alphapb.Reference{
				Spec: &storageproviderv0alphapb.Reference_Path{
					Path: target,
				},
			},
		}

		res, err := client.InitiateFileUpload(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		// TODO(labkode): upload to data server
		fmt.Printf("File will be uploaded to data server: %s\n", res.UploadEndpoint)

		dataServerURL := res.UploadEndpoint
		// TODO(labkode): do a protocol switch
		httpReq, err := http.NewRequest("PUT", dataServerURL, fd)
		if err != nil {
			return err
		}

		// TODO(labkode): harden http client
		// https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
		httpClient := &http.Client{
			Timeout: time.Second * 10,
		}

		httpRes, err := httpClient.Do(httpReq)
		if err != nil {
			return err
		}

		if httpRes.StatusCode != http.StatusOK {
			return err
		}

		req2 := &storageproviderv0alphapb.StatRequest{
			Ref: &storageproviderv0alphapb.Reference{
				Spec: &storageproviderv0alphapb.Reference_Path{
					Path: target,
				},
			},
		}
		res2, err := client.Stat(ctx, req2)
		if err != nil {
			return err
		}

		if res2.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		info := res2.Info

		fmt.Printf("Upload succeed: %s:%s %d %s\n", info.Id.StorageId, info.Id.OpaqueId, info.Size, info.Path)
		return nil
	}
	return cmd
}
