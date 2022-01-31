package main

import (
	"errors"
	"fmt"
	"io"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/gdexlab/go-render/render"
)

func getlockCommand() *command {
	cmd := newCommand("getlock")
	cmd.Description = func() string { return "get a lock on a resource" }
	cmd.Usage = func() string { return "Usage: getlock <resource_path>" }

	cmd.ResetFlags = func() {
		return
	}

	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 1 {
			return errors.New("Invalid arguments: " + cmd.Usage())
		}

		fn := cmd.Args()[0]
		client, err := getClient()
		if err != nil {
			return err
		}

		ctx := getAuthContext()

		ref := &provider.Reference{Path: fn}

		res, err := client.GetLock(ctx, &provider.GetLockRequest{
			Ref: ref,
		})

		if err != nil {
			return err
		}

		if res.Status.Code != rpc.Code_CODE_OK {
			return formatError(res.Status)
		}

		fmt.Println(render.Render(res.Lock))

		return nil
	}
	return cmd
}
