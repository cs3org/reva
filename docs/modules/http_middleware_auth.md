# HTTP Middleware: auth

This middleware authenticates requests to
HTTP services.

The logic is as follows: when a requests comes, the token strategy is triggered
to obtain an access token from the request. If a token is found, authenticaton
is not triggered. If a token is not found, the credentials strategy is
triggered to obtain user credentials (basic auth, OpenIDConnect, ...).
Then these credentials are validated against the authentication service
and if they are valid, an access token is obtained. This access token is written
to the response using a token writer strategy (reponse header, response cookie, ...).
Once the access token is obtained either because is set on the request or because
authentication was successful, the token is verified using the token manager 
strategy (jwt) to obtain the user context and pass it to outgoing requests.


To enable the middleware:

```
[http]
enabled_middlewares = ["auth"]
```

Example configuration:

```
[http.middlewares.auth]
authsvc = "0.0.0.0:9999"
credential_strategy = "basic"
token_strategy = "header"
token_writer = "header"
token_manager = "jwt"

[http.middlewares.auth.token_managers.jwt]
secret = "bar"

[http.middlewares.auth.token_strategies.header]
header = "X-Access-Token"

[http.middlewares.auth.token_writers.header]
header = "X-Access-Token"
```

## Directives

```
Syntax:  authsvc = string
Default: authsvc = "0.0.0.0:9999"
```
authsvc specifies the location of the authentication service.

```
Syntax:  credential_strategy = string
Default: credential_strategy = "basic"
```
credential_strategy specifies the strategy to use to obtain
user credentials.
Available strategies shipped with REVA can be consulted at the end of this section.
The default strategy is [Basic Auth](https://tools.ietf.org/html/rfc7617).

```
Syntax:  token_strategy = string
Default: token_strategy = "header"
```
token_strategy specifies the strategy to use to obtain
the access token from the HTTP request.
Available strategies shipped with REVA can be consulted at the end of this section.
The default strategy is obtain the token from an HTTP header.

```
Syntax:  token_writer = string
Default: token_writer = "header"
```
token_writer specifies the strategy to use write the 
access token once is obtained to the HTTP response so clients
can re-send it subsequent requests to avoid performing expensive authentication
calls to the authentication service.
Available writer strategies shipped with REVA can be consulted at the end of this section.
The default strategy is write the access token in an HTTP response header.

```
Syntax:  token_manager = string
Default: token_manager = "jwt"
```
token_manager specifies the strategy to use verify the access token.
Available token managers  shipped with REVA can be consulted at the end of this section.
The default manager is to verify it using JWT.
**The token manager configured for the authentication service and the token manager for 
this middleware MUST be the same**.


## Credential strategies

### Basic Authentication
This strategy obtains the credentials from Basic Auth.

To enable the strategy:

```
[http.middlewares.auth]
credential_strategy = "basic"
```

### OpenID Connect - **Work in Progress**
This strategy obtains the open id connect token as the credentials
that is passed to the authentication service to be verified 
agains the configured identity provider public keys.

To enable the strategy:

```
[http.middlewares.auth]
credential_strategy = "oidc"
```

## Token strategies

### Header
This token strategy obtains the access token from an HTTP request header.

To enable the strategy:

```
[http.middlewares.auth]
token_strategy = "header"
```
#### Directives

```
Syntax:  header = string
Default: header = ""
```
header specifies header name that contains the token.

## Token writers

### Header
This writer strategy writes the access token to an HTTP response header
specified by tbe **header** directive.

To enable the strategy:

```
[http.middlewares.auth]
token_writer = "header"

[http.middlewares.auth.token_writers.header]
header = "X-Access-Token"
```

#### Directives

```
Syntax:  header = string
Default: header = ""
```
header specifies header name to use to write the token.

## Token managers

### JWT
This token manager verifies the token using the JWT shared secret.

To enable the strategy:

```
[http.middlewares.auth]
token_manager = "jwt"

[http.middlewares.auth.token_managers.jwt]
secret = "bar"
```

#### Directives

```
Syntax:  secret = string
Default: secret = ""
```
secret specifies the shared secret to verify the JWT token.
