Change: Declare HTTP routes instead of handing the server one handler per service

Every HTTP service used to expose a single handler mounted under a prefix, and
route inside it however it liked: some with chi, some by splitting the path by
hand, one with a `ServeMux` of its own. The server matched the longest prefix,
stripped it, and passed the rest on, which is why four services had to undo
that stripping with `r.URL.RawPath = ""` before their router would match a
percent-encoded path.

Services now declare their routes on a `pkg/rhttp/router.Router`, built on the
standard library's `ServeMux`, and the server serves all of them from one
router. Patterns are absolute, so a request is matched once against the whole
server rather than once per prefix and again inside a service; methods are part
of the route, so a wrong method answers 405 instead of falling through; and a
handler that owns its URL space and cannot be restated as patterns - tus,
`pprof.Index`, WebDAV - is mounted, receiving the request path untouched.

The paths exempt from authentication are no longer a list each service keeps
next to, but separate from, its routing. A route declares itself unprotected,
and the server derives the list from the declared routes, so the two can no
longer drift apart.

`ocdav` used to be mounted at the root and so answered every unmatched URL. It
now declares the eight entry points it actually serves (`/dav`, `/webdav`,
`/remote.php`, `/status.php`, `/s`, `/apps/files`, `/index.php/s` and
`/ocm-provider`), and anything else gets an honest 404.

Change: Remove the configurable prefix of HTTP services

Each HTTP service took a `prefix` setting naming the path it was served under.
It was never really free: clients and federated peers have to agree on these
paths up front, and for most services the value is fixed by a specification -
`/.well-known/ocm` by the OCM spec, `/ocs/v1.php` and `/remote.php/dav` by the
ownCloud clients. `pprof` had already given up and overwrote whatever was
configured with `/debug`.

The prefix is now a constant per service. A `prefix` left in a config file is
ignored, so a deployment that set one to something other than the default has
to move its clients to the default path.

https://github.com/cs3org/reva/pull/5831
