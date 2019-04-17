package main

import (
	"fmt"
	"mime"
	"os"
	"path"

	appregistryv0alphapb "github.com/cernbox/go-cs3apis/cs3/appregistry/v0alpha"
	rpcpb "github.com/cernbox/go-cs3apis/cs3/rpc"
)

func appRegistryFindCommand() *command {
	cmd := newCommand("app-registry-find")
	cmd.Description = func() string {
		return "find applicaton provider for file extension or mimetype"
	}
	cmd.Action = func() error {
		if cmd.NArg() == 0 {
			fmt.Println(cmd.Usage())
			os.Exit(1)
		}

		fn := cmd.Args()[0]
		ext := path.Ext(fn)
		mime := mime.TypeByExtension(ext)
		req := &appregistryv0alphapb.GetAppProviderRequest{
			MimeType: mime,
		}

		client, err := getAppRegistryClient()
		if err != nil {
			return err
		}
		ctx := getAuthContext()
		res, err := client.GetAppProvider(ctx, req)
		if err != nil {
			return err
		}

		if res.Status.Code != rpcpb.Code_CODE_OK {
			return formatError(res.Status)
		}

		fmt.Printf("application provider can be found at %s\n", res.Provider.Address)
		return nil
	}
	return cmd
}
