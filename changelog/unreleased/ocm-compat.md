Bugfix: only expose paths on /ocm-provider

Following analysis of OC and NC code to access a remote share,
we must expose paths and not full URIs on the /ocm-provider endpoint.

https://github.com/cs3org/reva/pull/3962
