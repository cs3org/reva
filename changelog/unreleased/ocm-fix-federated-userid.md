Bugfix: fix OCM userid encoding

We now base64 encode the remote userid and provider as the local federated user id. This allows us to always differentiate them from local users and unpack the encoded user id and provider when making requests to the remote ocm provider.

https://github.com/cs3org/reva/pull/4833
https://github.com/owncloud/ocis/issues/9927
