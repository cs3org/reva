Bugfix: Trash Bin in oCIS Storage Operations

Support for restoring a target folder nested deep inside the trash bin in oCIS storage. The use case is:

```console
curl 'https://localhost:9200/remote.php/dav/trash-bin/einstein/f1/f2' -X MOVE -H 'Destination: https://localhost:9200/remote.php/dav/files/einstein/destination'
```

The previous command creates the `destination` folder and moves the contents of `/trash-bin/einstein/f1/f2` onto it.

Retro-compatibility in the response code with ownCloud 10. Restoring a collection to a non-existent nested target is not supported and MUST return `409`. The use case is:

```console
curl 'https://localhost:9200/remote.php/dav/trash-bin/einstein/f1/f2' -X MOVE -H 'Destination: https://localhost:9200/remote.php/dav/files/einstein/this/does/not/exist'
``` 

The previous command used to return `404` instead of the expected `409` by the clients.

https://github.com/cs3org/reva/pull/1926
