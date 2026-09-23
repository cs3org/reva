Enhancement: Add an HTTP gateway

A reva deployment spread over several processes needed a reverse proxy in front
to decide which process a request belongs to, configured with one location per
path prefix and kept in step with the services by hand.

The new `gateway` HTTP service is that entry point, and it is configured with
no paths at all. Every HTTP service advertises the routes it declared in the
service registry, and the gateway mirrors them into a router of the same kind
the services use, so a request is matched at the gateway exactly as it would be
at the service. The node serving a matched route is resolved per request, and
the request is forwarded with its path exactly as it arrived, which is what
WebDAV clients require. The gateway does not authenticate: the service it
forwards to applies its own auth to the route that was matched.

https://github.com/cs3org/reva/pull/5831
