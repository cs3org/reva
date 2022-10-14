Security: Mitigate XSS

We've mitigated an XSS vulnerability resulting from unescaped HTTP responses containing
user-provided values in pkg/siteacc/siteacc.go and internal/http/services/ocmd/invites.go.
This patch uses html.EscapeString to escape the user-provided values in the HTTP
responses of pkg/siteacc/siteacc.go and internal/http/services/ocmd/invites.go.

https://github.com/cs3org/reva/pull/3316
