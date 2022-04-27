Bugfix: Disable storageprovider cache for the share jail

The share jail should not be cached in the provider cache because it is a virtual collection of resources from different storage providers.

https://github.com/cs3org/reva/pull/2784
