Enhancement: add served webapp launcher page

ScienceMesh now serves `POST /sciencemesh/webapp-launch`, a protected route
that validates the received webapp share the same way `/open-in-app` does,
exchanges the shared secret for an access token, and renders a small page that
auto-submits a form posting `access_token` to the validated app URL. Nothing
is stored on the server and no unprotected route is added. Every response sets
`Content-Security-Policy` plus `Cache-Control: no-store`,
`Referrer-Policy: no-referrer` and `X-Content-Type-Options: nosniff`. On the
launcher page `form-action` is the exact validated app origin for standard
hosts, with a safe fixed `https:` fallback for unusual validated hosts; error
pages use `form-action 'none'`. Error pages carry fixed text only. An
`expired_session_redirect_uri` is forwarded only when its origin is
`https://<provider_domain>`; otherwise it is dropped.
