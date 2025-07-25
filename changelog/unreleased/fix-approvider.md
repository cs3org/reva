Bugfix: approvider failed to create files

In https://github.com/cs3org/reva/pull/4864, a new header was introduced for sending content lengths to the datagateway. This header was missing in the appprovider, causing it to fail when creating new files.

https://github.com/cs3org/reva/pull/5236