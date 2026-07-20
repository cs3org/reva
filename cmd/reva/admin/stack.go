// Copyright 2018-2026 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/cs3org/reva/v3/pkg/admin/adminpb"
)

func adminStackCommand() *command {
	cmd := newCommand("stack")
	cmd.Description = func() string { return "dump the goroutine stacks of the selected instances" }
	cmd.Usage = func() string { return "Usage: admin stack [-admin-host h] [-grep P] <selector>" }
	adminHost := cmd.String("admin-host", "", "address of the admin gRPC endpoint (persisted)")
	grep := cmd.String("grep", "", "keep only goroutines whose stack contains this substring")
	cmd.ResetFlags = func() { *adminHost, *grep = "", "" }
	cmd.Action = func(w ...io.Writer) error {
		if cmd.NArg() < 1 {
			return errors.New(cmd.Usage())
		}
		args := map[string]string{}
		if *grep != "" {
			args["grep"] = *grep
		}
		client, ctx, err := adminDial(*adminHost)
		if err != nil {
			return err
		}
		// Stream per instance: a dump is large, so one aggregate response could
		// exceed the message limit on a wide fan-out.
		stream, err := client.InvokeStream(ctx, &adminpb.InvokeRequest{Service: cmd.Args()[0], Invocation: "stack", Args: args})
		if err != nil {
			return adminErr(err)
		}
		first := true
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return adminErr(err)
			}
			if !first {
				fmt.Println()
			}
			first = false
			if msg.Error != "" {
				fmt.Printf("# %s: error: %s\n", msg.Node, msg.Error)
				continue
			}
			var d struct {
				Goroutines int    `json:"goroutines"`
				HeapAlloc  uint64 `json:"heap_alloc_bytes"`
				NumGC      uint32 `json:"num_gc"`
				Stacks     string `json:"stacks"`
				Truncated  bool   `json:"truncated"`
			}
			if err := json.Unmarshal([]byte(msg.ResultJson), &d); err != nil {
				fmt.Printf("# %s: %s\n", msg.Node, msg.ResultJson)
				continue
			}
			fmt.Printf("# %s  goroutines=%d heap=%.1fMiB gc=%d\n", msg.Node, d.Goroutines, float64(d.HeapAlloc)/(1<<20), d.NumGC)
			fmt.Println(d.Stacks)
			if d.Truncated {
				fmt.Println("# (dump truncated)")
			}
		}
	}
	return cmd
}
