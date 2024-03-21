Enhancement: Allow passing a initiator id

Allows passing an initiator id on http request as `Initiator-ID` header. It will be passed down though ocis
and returned with sse events (clientlog events, as userlog has its own logic)

https://github.com/cs3org/reva/pull/4587
