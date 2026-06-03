Bugfix: Bound the Prometheus HTTP handler label cardinality

The HTTP metrics interceptor used the full request URL path as the value of the
`handler` label on the `http_request_duration_seconds` histogram. Because Reva
HTTP endpoints are user-bound, this produced unbounded label cardinality and
caused excessive metrics storage. The label is now derived from the leading
static path segment, keeping cardinality bounded to the number of route
prefixes.

https://github.com/cs3org/reva/issues/4509
