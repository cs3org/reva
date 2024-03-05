Enhancement: Allow tracing requests by giving util functions a context

We deprecated GetServiceUserContext with GetServiceUserContextWithContext and GetUser with GetUserWithContext to allow passing in a trace context.

https://github.com/cs3org/reva/pull/4556