// Copyright 2018-2024 CERN
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

package grpc_test

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/cs3org/reva/v3/pkg/registry"
	_ "github.com/cs3org/reva/v3/pkg/registry/loader" // register memory + nats drivers
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// natsAddr is the NATS endpoint used by the registry-registration suite. The
// dev environment (../reva-dev) exposes JetStream on localhost:4222; override
// with REVA_TEST_NATS_ADDRESS if needed.
func natsAddr() string {
	if a := os.Getenv("REVA_TEST_NATS_ADDRESS"); a != "" {
		return a
	}
	return "nats://localhost:4222"
}

func natsReachable() bool {
	host := "localhost:4222"
	if a := os.Getenv("REVA_TEST_NATS_ADDRESS"); a != "" {
		// strip scheme if present
		host = a
		for _, p := range []string{"nats://", "tls://"} {
			if len(host) > len(p) && host[:len(p)] == p {
				host = host[len(p):]
			}
		}
	}
	c, err := net.DialTimeout("tcp", host, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// This suite verifies that a revad correctly self-registers every service it
// loads into the shared registry, with the expected framework- and
// service-owned metadata. It boots one revad against a NATS-backed registry
// (a per-run bucket so it does not collide with the dev fleet), then reads the
// bucket back through an independent registry client and asserts on it.
var _ = Describe("service self-registration", func() {
	var (
		revads map[string]*Revad
		reg    registry.Registry
		bucket string
	)

	BeforeEach(func() {
		if !natsReachable() {
			Skip("NATS is not reachable on localhost:4222 (start ../reva-dev or set REVA_TEST_NATS_ADDRESS)")
		}
	})

	JustBeforeEach(func() {
		var err error
		bucket = fmt.Sprintf("reva_registry_test_%d_%d", os.Getpid(), time.Now().UnixNano())

		revads, err = startRevads(map[string]string{
			"node": "registry-registration.toml",
		}, nil, nil, map[string]string{
			"nats_address":    natsAddr(),
			"registry_bucket": bucket,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(revads["node"]).ToNot(BeNil())

		// Independent client over the same bucket; WatchAll replays existing keys
		// into the local cache, so give it a moment to hydrate.
		reg, err = registry.New("nats", map[string]any{
			"address": natsAddr(),
			"bucket":  bucket,
		}, registry.Thresholds{})
		Expect(err).ToNot(HaveOccurred())
		Eventually(func() int {
			svcs, err := reg.ListServices()
			if err != nil {
				return 0
			}
			return len(svcs)
		}, 10*time.Second, 200*time.Millisecond).Should(BeNumerically(">=", 5))
	})

	AfterEach(func() {
		for _, r := range revads {
			Expect(r.Cleanup(CurrentGinkgoTestDescription().Failed)).To(Succeed())
		}
	})

	nodeFor := func(name string) registry.Node {
		svc, err := reg.GetService(name)
		Expect(err).ToNot(HaveOccurred(), "service %q not registered", name)
		Expect(svc.Nodes()).ToNot(BeEmpty(), "service %q has no nodes", name)
		return svc.Nodes()[0]
	}

	It("registers every loaded gRPC and HTTP service", func() {
		for _, name := range []string{
			"storageprovider", "authprovider", "userprovider", // gRPC
			"dataprovider", "datagateway", // HTTP
		} {
			_, err := reg.GetService(name)
			Expect(err).ToNot(HaveOccurred(), "expected %q to be registered", name)
		}
	})

	It("stamps the framework metadata on every node", func() {
		for _, tc := range []struct {
			name      string
			transport string
		}{
			{"storageprovider", "grpc"},
			{"authprovider", "grpc"},
			{"dataprovider", "http"},
			{"datagateway", "http"},
		} {
			n := nodeFor(tc.name)
			md := n.Metadata()
			Expect(n.Address()).ToNot(BeEmpty(), "%s: address", tc.name)
			Expect(n.ID()).ToNot(BeEmpty(), "%s: id", tc.name)
			Expect(md["transport"]).To(Equal(tc.transport), "%s: transport", tc.name)
			Expect(md["host"]).ToNot(BeEmpty(), "%s: host", tc.name)
			Expect(md["pid"]).ToNot(BeEmpty(), "%s: pid", tc.name)
			Expect(md[registry.MetaState]).To(Equal(registry.StateReady), "%s: state", tc.name)
			Expect(md[registry.MetaLastSeen]).ToNot(BeEmpty(), "%s: last_seen", tc.name)
		}
	})

	It("adds scheme and prefix for HTTP services only", func() {
		// HTTP services advertise scheme + prefix.
		for name, prefix := range map[string]string{
			"dataprovider": "data",
			"datagateway":  "datagateway",
		} {
			md := nodeFor(name).Metadata()
			Expect(md[registry.MetaScheme]).To(Equal("http"), "%s: scheme", name)
			Expect(md[registry.MetaPrefix]).To(Equal(prefix), "%s: prefix", name)
		}
		// gRPC services do not.
		md := nodeFor("storageprovider").Metadata()
		Expect(md[registry.MetaScheme]).To(BeEmpty(), "storageprovider should not advertise scheme")
		Expect(md[registry.MetaPrefix]).To(BeEmpty(), "storageprovider should not advertise prefix")
	})

	It("merges service-owned metadata via MetadataProvider", func() {
		// the data provider advertises mount_id + public_url.
		dp := nodeFor("dataprovider").Metadata()
		Expect(dp[registry.MetaMountID]).To(Equal("registration-mount-id"))
		Expect(dp[registry.MetaPublicURL]).To(Equal("http://" + revads["node"].HTTPAddress + "/data"))

		// the data gateway advertises its public_url.
		dg := nodeFor("datagateway").Metadata()
		Expect(dg[registry.MetaPublicURL]).To(Equal("http://" + revads["node"].HTTPAddress + "/datagateway"))
	})
})
