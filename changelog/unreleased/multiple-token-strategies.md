Enhancement: Support multiple token strategies in auth middleware

Different HTTP services can in general support different
token strategies for validating the reva token.
In this context, without updating every single client 
a mono process deployment will never work.
Now the HTTP auth middleware accepts in its configuration a
token strategy chain, allowing to provide the reva
token in multiple places (bearer auth, header).

https://github.com/cs3org/reva/pull/4030
