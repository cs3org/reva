Change: Change the oCIS and S3NG  storage driver blob store layout

We've optimized the oCIS and S3NG storage driver blob store layout.

For the oCIS storage driver, blobs will now be stored inside the folder
of a space, next to the nodes. This allows admins to easily archive, backup and restore
spaces as a whole with UNIX tooling. We also moved from a single folder for blobs to
multiple folders for blobs, to make the filesystem interactions more performant for
large numbers of files.

The previous layout on disk looked like this:

```
|-- spaces
| |-- ..
| |  |-- ..
| |-- xx
|   |-- xxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx <- partitioned space id
|     |-- nodes
|       |-- ..
|       |-- xx
|         |-- xx
|           |-- xx
|             |-- xx
|               |-- -xxxx-xxxx-xxxx-xxxxxxxxxxxx <- partitioned node id
|-- blobs
  |-- ..
  |-- xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx <- blob id
```

Now it looks like this:

```
|-- spaces
| |-- ..
| |  |-- ..
  |-- xx
    |-- xxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx <- partitioned space id
      |-- nodes
      |  |-- ..
      |  |-- xx
      |    |-- xx
      |      |-- xx
      |        |-- xx
      |          |-- -xxxx-xxxx-xxxx-xxxxxxxxxxxx <- partitioned node id
      |-- blobs
        |-- ..
        |-- xx
          |-- xx
            |-- xx
              |-- xx
                |-- -xxxx-xxxx-xxxx-xxxxxxxxxxxx <- partitioned blob id
```

For the S3NG storage driver, blobs will now be prefixed with the space id and
also a part of the blob id will be used as prefix. This creates a better prefix partitioning
and mitigates S3 api performance drops for large buckets (https://aws.amazon.com/de/premiumsupport/knowledge-center/s3-prefix-nested-folders-difference/).

The previous S3 bucket (blobs only looked like this):

```
|-- ..
|-- xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx <- blob id
```

Now it looks like this:

```
|-- ..
|-- xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx <- space id
   |-- ..
   |-- xx
     |-- xx
       |-- xx
         |-- xx
           |-- -xxxx-xxxx-xxxx-xxxxxxxxxxxx <- partitioned blob id
```

https://github.com/cs3org/reva/pull/2763
https://github.com/owncloud/ocis/issues/3557
