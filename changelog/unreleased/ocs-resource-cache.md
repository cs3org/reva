Enhancement: Cache resources from share getter methods in OCS

In OCS, once we retrieve the shares from the shareprovider service, we stat each
of those separately to obtain the required info, which introduces a lot of
latency. This PR introduces a resoource info cache in OCS, which would prevent
this latency.

https://github.com/cs3org/reva/pull/1643
