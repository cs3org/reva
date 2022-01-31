package main

import (
	"errors"
	"fmt"
	"io"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
)

func unlockCommand() *command {
	cmd := newCommand("unlock")
	cmd.Description = func() string { return "remove a lock on a resource" }
	cmd.Usage = func() string { return "Usage: unlock <resource_path>" }

	idFlag := cmd.String("id", "", "lock id")

	cmd.ResetFlags = func() {
		*idFlag = ""
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

		// get lock from the id if set
		var lock *provider.Lock
		if *idFlag == "" {
			getLockRes, err := client.GetLock(ctx, &provider.GetLockRequest{
				Ref: ref,
			})
			if err != nil {
				return err
			}
			if getLockRes.Status.Code != rpc.Code_CODE_OK {
				return formatError(getLockRes.Status)
			}
			lock = getLockRes.Lock
		} else {
			lock = &provider.Lock{
				LockId: *idFlag,
			}
		}

		res, err := client.Unlock(ctx, &provider.UnlockRequest{
			Ref:  ref,
			Lock: lock,
		})

		if err != nil {
			return err
		}

		if res.Status.Code != rpc.Code_CODE_OK {
			return formatError(res.Status)
		}

		fmt.Println("OK")

		return nil
	}
	return cmd
}
