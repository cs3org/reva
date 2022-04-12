Bugfix: Ensure that the host in the ocs config endpoint has no protocol

We've fixed the host info in the ocs config endpoint so that it has no protocol, as ownCloud 10 doesn't have it.

https://github.com/cs3org/reva/pull/2692
https://github.com/owncloud/ocis/pull/3113
