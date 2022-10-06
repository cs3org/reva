Security: Mitigate XSS

We've mitigated an XSS vulnerability resulting from not sanitising the HTTP requests.
net/http provides a router – ServeMux, which does more than routing, it also sanitises
the requests.
Instead of using ServeMux we were directly using an http.Handler which routes the
request based on the URL.Path without sanitizing it.
Besides, in pkg/siteacc/siteacc.go and internal/http/services/ocmd/invites.go we were
creating http responses with user-provided values.
This patch adds a http.ServeMux to sanitise the request before reaching any other
handler and uses html.EscapeString to sanitise the user-provided values in the http
responses of pkg/siteacc/siteacc.go and internal/http/services/ocmd/invites.go.

https://github.com/cs3org/reva/pull/3316
