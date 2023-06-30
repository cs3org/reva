// Copyright 2018-2023 CERN
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

package net

import "net"

// AddressEqual return true if the addresses are equal.
// For tpc addressess only the port is compared, for unix
// the name and net are compared.
func AddressEqual(a net.Addr, network, address string) bool {
	if a.Network() != network {
		return false
	}

	switch network {
	case "tcp":
		t, err := net.ResolveTCPAddr(network, address)
		if err != nil {
			return false
		}
		return tcpAddressEqual(a.(*net.TCPAddr), t)
	case "unix":
		t, err := net.ResolveUnixAddr(network, address)
		if err != nil {
			return false
		}
		return unixAddressEqual(a.(*net.UnixAddr), t)
	}
	return false
}

func tcpAddressEqual(a1, a2 *net.TCPAddr) bool {
	return a1.Port == a2.Port
}

func unixAddressEqual(a1, a2 *net.UnixAddr) bool {
	return a1.Name == a2.Name && a1.Net == a2.Net
}
