Enhancement: upload directly to dataprovider

The ocdav service can now bypass the datagateway if it is configured with a transfer secret. This prevents unnecessary roundtrips and halves the network traffic during uploads for the proxy.

https://github.com/cs3org/reva/pull/4065
https://github.com/owncloud/ocis/issues/6296