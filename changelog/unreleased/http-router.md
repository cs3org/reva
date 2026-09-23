Change: Let HTTP services declare the routes they serve

Every HTTP service used to hand the server a single handler mounted under a
configurable prefix, and route inside it however it liked: some with chi, some
by splitting the path by hand. Services now declare their routes on a router
built on the standard library's ServeMux, and the server serves all of them
from one table.

Patterns are absolute, so a request is matched once instead of once per prefix
and again inside a service. Methods are part of the route, so a wrong method
answers 405. The paths exempt from authentication are declared on the routes
themselves and read off the matched route, rather than kept in a separate list
that could drift from what is actually served.

Two consequences for existing configurations. The `prefix` setting of every
HTTP service is gone: the path is now a constant, and a `prefix` left in a
config file is ignored, so a deployment that set one to a non-default value has
to move its clients. And `ocdav`, which was mounted at the root and therefore
answered every unmatched URL, now declares the eight entry points it actually
serves, so anything else gets a 404.

https://github.com/cs3org/reva/pull/5831
