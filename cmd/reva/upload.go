package main

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/cernbox/reva/cmd/revad/svcs/grpcsvcs/storageprovidersvc"

	"github.com/cernbox/reva/pkg/crypto"

	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
	storageproviderv0alphapb "github.com/cernbox/go-cs3apis/cs3/storageprovider/v0alpha"
	"github.com/cheggaaa/pb"
)

func uploadCommand() *command {
	cmd := newCommand("upload")
	cmd.Description = func() string { return "upload a local file to the remote server" }
	xsFlag := cmd.String("xs", "negotiate", "compute checksum")
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
		defer fd.Close()

		md, err := fd.Stat()
		if err != nil {
			return err
		}
		defer fd.Close()

		fmt.Printf("Local file size: %d bytes\n", md.Size())

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
		fmt.Printf("Data server: %s\n", res.UploadEndpoint)
		fmt.Printf("Allowed checksums: %+v\n", res.AvailableChecksums)

		xsType, err := guessXS(*xsFlag, res.AvailableChecksums)
		if err != nil {
			return err
		}
		fmt.Printf("Checksum selected: %s\n", xsType)

		xs, err := computeXS(xsType, fd)
		if err != nil {
			return err
		}

		fmt.Printf("Local XS: %s:%s\n", xsType, xs)
		// seek back reader to 0
		if _, err := fd.Seek(0, 0); err != nil {
			return err
		}

		dataServerURL := res.UploadEndpoint
		bar := pb.New(int(md.Size())).SetUnits(pb.U_BYTES)
		bar.Start()
		reader := bar.NewProxyReader(fd)

		// TODO(labkode): do a protocol switch
		httpReq, err := http.NewRequest("PUT", dataServerURL, reader)
		if err != nil {
			return err
		}

		q := httpReq.URL.Query()
		q.Add("xs", xs)
		q.Add("xs_type", storageprovidersvc.GRPC2PKGXS(xsType))
		httpReq.URL.RawQuery = q.Encode()

		// TODO(labkode): harden http client
		// https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
		httpClient := &http.Client{
			Timeout: time.Second * 10,
		}

		httpRes, err := httpClient.Do(httpReq)
		if err != nil {
			return err
		}

		bar.Finish()

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

		fmt.Printf("File uploaded: %s:%s %d %s\n", info.Id.StorageId, info.Id.OpaqueId, info.Size, info.Path)
		return nil
	}
	return cmd
}

func computeXS(t storageproviderv0alphapb.ResourceChecksumType, r io.Reader) (string, error) {
	switch t {
	case storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_ADLER32:
		return crypto.ComputeAdler32XS(r)
	case storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_MD5:
		return crypto.ComputeMD5XS(r)
	case storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_SHA1:
		return crypto.ComputeSHA1XS(r)
	case storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_UNSET:
		return "", nil
	default:
		return "", fmt.Errorf("invalid checksum: %s", t)

	}
}

func guessXS(xsFlag string, availableXS []*storageproviderv0alphapb.ResourceChecksumPriority) (storageproviderv0alphapb.ResourceChecksumType, error) {
	// force use of cheksum if available server side.
	if xsFlag != "negotiate" {
		wanted := storageprovidersvc.PKG2GRPCXS(xsFlag)
		if wanted == storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID {
			return storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID, fmt.Errorf("desired checksum is invalid: %s", xsFlag)
		}
		if isXSAvailable(wanted, availableXS) {
			return wanted, nil
		}
		return storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID, fmt.Errorf("checksum not available server side: %s", xsFlag)
	}

	// negotiate the checksum type based on priority list from server-side.
	if len(availableXS) == 0 {
		return storageproviderv0alphapb.ResourceChecksumType_RESOURCE_CHECKSUM_TYPE_INVALID, fmt.Errorf("no available xs for negotiating")
	}

	// pick the one with priority to the lower number.
	desired := pickChecksumWithHighestPrio(availableXS)
	return desired, nil
}

func pickChecksumWithHighestPrio(xss []*storageproviderv0alphapb.ResourceChecksumPriority) storageproviderv0alphapb.ResourceChecksumType {
	var chosen storageproviderv0alphapb.ResourceChecksumType
	var maxPrio uint32 = math.MaxUint32
	for _, xs := range xss {
		if xs.Priority < maxPrio {
			maxPrio = xs.Priority
			chosen = xs.Type
		}
	}
	return chosen
}

func isXSAvailable(t storageproviderv0alphapb.ResourceChecksumType, available []*storageproviderv0alphapb.ResourceChecksumPriority) bool {
	for _, xsPrio := range available {
		if xsPrio.Type == t {
			return true
		}
	}
	return false
}
