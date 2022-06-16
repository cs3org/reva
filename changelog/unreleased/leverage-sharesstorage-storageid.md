Enhancement: Leverage shares space storageid and type when listing shares

The list shares call now also fills the storageid to allow the space registry to directly route requests to the correct storageprovider. The spaces registry will now also skip storageproviders that are not configured for a requested type, causing type 'personal' requests to skip the sharestorageprovider.

https://github.com/cs3org/reva/pull/2975
https://github.com/cs3org/reva/pull/2980