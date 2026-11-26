Bugfix: Report restore collisions on EOS

A restore onto an existing path looked like a success in the gRPC client
and gave a generic error in the binary client, so the item silently
stayed in the bin. A forced restore now keeps both items, as EOS does.

https://github.com/cs3org/reva/pull/6051