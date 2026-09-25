Change: Declare HTTP routes, and route between services with a gateway

HTTP services used to hand the server one handler mounted under a configurable
prefix, and route inside it however they liked. They now declare their routes
on a router built on the standard library's ServeMux, and the server serves all
of them from one table: patterns are absolute, methods are part of the route,
and the paths exempt from authentication are declared on the routes themselves
rather than kept in a separate list that could drift.

Each service advertises its routes in the service registry, which is what the
new `gateway` HTTP service mirrors: a single entry point that forwards a
request to the service owning the route it matches, configured with no paths at
all. It resolves the serving node per request and forwards the path exactly as
it arrived, as WebDAV clients require, leaving authentication to the service.

Two consequences for existing configurations. The `prefix` setting of every
HTTP service is gone, so a deployment that set one to a non-default value has
to move its clients. And `ocdav`, previously mounted at the root and therefore
answering every unmatched URL, now declares the entry points it actually
serves, so anything else gets a 404.

Services no longer expose a prefix at all: the routes say where they serve. The
two whose URL a peer builds from the registry, the data provider and the data
gateway, advertise the path they are served under themselves.

The WebDAV service used to route itself, walking the request path segment by
segment through nested switches spread over several files, accumulating the
base URI as it went and rewriting the URL in place. It now declares its entry
points, each with the base URI it reports fixed at declaration and a handler
declared per method, so the router picks the operation and refuses a method
nobody serves. What is left of the walk is one segment read per subtree, for
the user, space, share or token its URL carries.

https://github.com/cs3org/reva/pull/5831
