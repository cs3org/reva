# HTTP Middleware: cors

The cors middleware takes care of CORS headers. It is needed to allow authenticating users using oidc, which will make cross origin resource POST requests when using the recommended autorization code flow.

If you hide the idp, phoenix and reva behind a reverse proxy and serve them all from the same domain you may be able to disable it.

To enable the middleware:

```
[http]
enabled_middlewares = ["cors"]
```

Example configuration:

```
[http.middlewares.cors]
allowed_origins = ["*"] # allow requests from everywhere
allowed_methods = ["OPTIONS", "GET", "PUT", "POST", "DELETE", "MKCOL", "PROPFIND", "PROPPATCH", "MOVE", "COPY", "REPORT", "SEARCH"]
allowed_headers = ["Origin", "Accept", "Depth", "Content-Type", "X-Requested-With", "Authorization", "Ocs-Apirequest", "If-None-Match"]
allow_credentials = true
options_passthrough = false
```
