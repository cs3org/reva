Enhancement: Add support for multiple data transfer protocols

Previously, we had to configure which data transfer protocol to use in the
dataprovider service. A previous PR added the functionality to redirect requests
to different handlers based on the request method but that would lead to
conflicts if multiple protocols don't support mutually exclusive sets of
requests. This PR adds the functionality to have multiple such handlers
simultaneously and the client can choose which protocol to use.

https://github.com/cs3org/reva/pull/1321
https://github.com/cs3org/reva/pull/1285/
