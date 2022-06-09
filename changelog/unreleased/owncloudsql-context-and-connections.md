Enhancement: improve owncloudsql connection management

The owncloudsql storagedriver is now aware of the request context and will close db connections when http connections are closed or time out. We also increased the max number of open connections from 10 to 100 to prevent a corner case where all connections were used but idle connections were not freed.

https://github.com/cs3org/reva/pull/2944