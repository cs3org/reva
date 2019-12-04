# GRPC interceptor: auth

This interceptor authenticates requests to
GRPC services.

To enable the interceptor:

```
[grpc]
enabled_interceptors = ["auth"]
```

Example configuration:

```
[grpc.interceptors.auth]
token_manager = "jwt"
#header   = "x-access-token"
skip_methods = [
    # allow calls that happen during authentication
	"/cs3.gatewayv0alpha.GatewayService/Authenticate",
	"/cs3.gatewayv0alpha.GatewayService/WhoAmI",
	"/cs3.gatewayv0alpha.GatewayService/GetUser",
	"/cs3.gatewayv0alpha.GatewayService/ListAuthProviders",
	"/cs3.authregistryv0alpha.AuthRegistryService/ListAuthProviders",
	"/cs3.authregistryv0alpha.AuthRegistryService/GetAuthProvider",
	"/cs3.authproviderv0alpha.AuthProviderService/Authenticate",
	"/cs3.userproviderv0alpha.UserProviderService/GetUser",
]

[grpc.interceptors.auth.token_managers.jwt]
secret = "Pive-Fumkiu4"
```

## Directives

```
Syntax:  token_manager = string
Default: token_manager = "jwt"
```
token_manager specifies the strategy to use verify the access token.
Available token managers  shipped with REVA can be consulted at the end of this section.
The default manager is to verify it using JWT.
**The token manager configured for the authentication service and the token manager for 
this middleware MUST be the same**.

TODO: header
TODO: skip_methods
