/*
 * MIT License
 *
 * Copyright (c) 2020 Daniel Mueller
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package net

import (
	"fmt"

	rpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
)

type rpcStatusGetter interface {
	GetStatus() *rpc.Status
}

// CheckRPCInvocation checks if an RPC invocation has succeeded.
// For this, the error from the original call is first checked; after that, the actual RPC response status is checked.
func CheckRPCInvocation(operation string, res rpcStatusGetter, callErr error) error {
	if callErr != nil {
		return fmt.Errorf("%s: %v", operation, callErr)
	}

	return CheckRPCStatus(operation, res)
}

// CheckRPCStatus checks the returned status of an RPC call.
func CheckRPCStatus(operation string, res rpcStatusGetter) error {
	status := res.GetStatus()
	if status.Code != rpc.Code_CODE_OK {
		return fmt.Errorf("%s: %q (code=%+v, trace=%q)", operation, status.Message, status.Code, status.Trace)
	}

	return nil
}
