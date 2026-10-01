Bugfix: Wrong checksum type and value reported on EOS

With the gRPC client the checksum type came out as INVALID and the value carried
56 zeros of padding, so a client comparing checksums never matched.

https://github.com/cs3org/reva/pull/5862
