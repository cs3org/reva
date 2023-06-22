---
title: "Data transfer functionality in Reva"
linkTitle: "Data transfer functionality"
weight: 5
description: >
  Data transfer functionality in Reva.
---

This is a guide on how to try the data transfer functionality in Reva in your local environment using rclone as the data transfer driver.

### Recap
A data transfer is initiated through an OCM share by setting the `protocol` to type `datatx`. 

## Prerequisites
* Have an rclone instance running (see [Rclone setup](#1-rclone-setup) below).
* A mesh setup equal to the OCM share tutorial (see [Reva daemons setup](#2-reva-daemons-setup)).

## 1. Rclone setup
Use rclone version v1.61 or higher. Available at [https://rclone.org/](https://rclone.org/).
<br>The rclone server should be run with the `server-side-across-configs` flag set to `true` which will make HTTP Third Party Copy (TPC) transfers possible:
```
rclone -vv rcd --server-side-across-configs=true --rc-user=rclone --rc-pass=rclonesecret --rc-addr=0.0.0.0:5572
```
TPC allows for direct (ie. efficient) Reva to Reva transfers as opposed to streaming the data through rclone

## 2. Reva daemons setup
Follow the setup ([prerequisites](https://reva.link/docs/tutorials/share-tutorial/#prerequisites), [building](https://reva.link/docs/tutorials/share-tutorial/#2-build-reva), [running](https://reva.link/docs/tutorials/share-tutorial/#3-run-reva)) of the OCM share [tutorial](https://reva.link/docs/tutorials/share-tutorial/).

Use the [data transfer example config](https://github.com/cs3org/reva/blob/master/examples/datatx/datatx.toml) for the relevant settings to enable rclone driven data transfer.

At this point you should have a two Reva daemon setup between which we will establish a data transfer driven by rclone.

## 3. Create a datatx protocol type OCM share
(assume we are logged in as einstein on the first Reva instance and we have uploaded some data to the `/home/my-data` folder)
<br>The tutorial explains transfer between user einstein at cern and user marie at cesnet.

Creating a transfer is similar to creating a regular OCM share through the `ocm-share-create` command with the addition of the `-datatx` flag. The `-datatx` flag signifies that this is a data transfer. 
<br>The `ocm-share-create` command makes (see example below), via an OCM share, the contents of folder `/home/my-data` available for transferring to the grantee.
<br>*Note that only a folder can be transferred!
```
>> ocm-share-create -grantee f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c -idp cesnet.cz -transfer /home/my-data
+--------------------------------------+-----------------+--------------------------------------+--------------------------------------------------------------------------------------------+-------------------+-------------+--------------------------------------+--------------------------------+--------------------------------+
| #                                    | OWNER.IDP       | OWNER.OPAQUEID                       | RESOURCEID                                                                                 | TYPE              | GRANTEE.IDP | GRANTEE.OPAQUEID                     | CREATED                        | UPDATED                        |
+--------------------------------------+-----------------+--------------------------------------+--------------------------------------------------------------------------------------------+-------------------+-------------+--------------------------------------+--------------------------------+--------------------------------+
| edc8f1c3-5f12-4430-8680-95b9034d6592 | cernbox.cern.ch | 4c510ada-c86b-4815-8820-42cdf82c3d51 | storage_id:"123e4567-e89b-12d3-a456-426655440000" opaque_id:"fileid-einstein%2Fmy-data"  | GRANTEE_TYPE_USER | cesnet.cz   | f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c | 2023-04-11 11:52:08 +0200 CEST | 2023-04-11 11:52:08 +0200 CEST |
+--------------------------------------+-----------------+--------------------------------------+--------------------------------------------------------------------------------------------+-------------------+-------------+--------------------------------------+--------------------------------+--------------------------------+
```

## 4. Discovering the transfer
(assume we are logged in on the receiving Reva instance as marie)
<br>
<br>The grantee (ie. the receiver of the transfer) can now discover the transfer share and its details in the same way as with regular shares using the `ocm-share-list-received` command to obtain the share id, and subsequent `ocm-share-get-received` command using that share id:

```
>> ocm-share-list-received
+--------------------------------------+-----------------+--------------------------------------+-------------------------------------------------------------------------------+-------------------+-------------+--------------------------------------+--------------------------------+--------------------------------+---------------------+-----------------+
| #                                    | OWNER.IDP       | OWNER.OPAQUEID                       | RESOURCEID                                                                    | TYPE              | GRANTEE.IDP | GRANTEE.OPAQUEID                     | CREATED                        | UPDATED                        | STATE               | SHARETYPE       |
+--------------------------------------+-----------------+--------------------------------------+-------------------------------------------------------------------------------+-------------------+-------------+--------------------------------------+--------------------------------+--------------------------------+---------------------+-----------------+
| 79a2bf32-4bba-437a-ad8f-ec93211375b5 | cernbox.cern.ch | 4c510ada-c86b-4815-8820-42cdf82c3d51 | opaque_id:"123e4567-e89b-12d3-a456-426655440000:fileid-einstein%2Fmy-data"  | GRANTEE_TYPE_USER | cesnet.cz   | f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c | 2023-04-11 11:52:08 +0200 CEST | 2023-04-11 11:52:08 +0200 CEST | SHARE_STATE_PENDING | SHARE_TYPE_USER |
+--------------------------------------+-----------------+--------------------------------------+-------------------------------------------------------------------------------+-------------------+-------------+--------------------------------------+--------------------------------+--------------------------------+---------------------+-----------------+

>> ocm-share-get-received 79a2bf32-4bba-437a-ad8f-ec93211375b5
{"id":{"opaqueId":"79a2bf32-4bba-437a-ad8f-ec93211375b5"}, "name":"my-data", "resourceId":{"opaqueId":"123e4567-e89b-12d3-a456-426655440000:fileid-einstein%2Fmy-data"}, "grantee":{"type":"GRANTEE_TYPE_USER", "userId":{"idp":"cesnet.cz", "opaqueId":"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c"}}, "owner":{"idp":"cernbox.cern.ch", "opaqueId":"4c510ada-c86b-4815-8820-42cdf82c3d51", "type":"USER_TYPE_FEDERATED"}, "creator":{"idp":"cernbox.cern.ch", "opaqueId":"4c510ada-c86b-4815-8820-42cdf82c3d51", "type":"USER_TYPE_FEDERATED"}, "ctime":{"seconds":"1683549473", "nanos":722800878}, "mtime":{"seconds":"1683549473", "nanos":722800878}, "shareType":"SHARE_TYPE_USER", "protocols":[{"transferOptions":{"sourceUri":"https://cernbox.cern.ch/remote.php/dav/ocm/IFs4ZVKVjp7OQsArvCSvXkf8A7emEQ71"}}], "state":"SHARE_STATE_PENDING", "resourceType":"RESOURCE_TYPE_CONTAINER"}
```
To start the transfer it must be accepted by the grantee.

## 4. Accepting the transfer by the grantee
The grantee (ie. the receiver of the transfer) must now accept the transfer by updating the `state` of the transfer to `accepted`. That will start the transfer. Optionally the grantee can also specify a path to which the data must be transferred:

```
>> ocm-share-update-received -state accepted -path /home/transfers 79a2bf32-4bba-437a-ad8f-ec93211375b5
OK
```
At this point the transfer should have started automatically. In the command example above the data will be transferred into the `/home/transfers` folder of the grantee. In this case the final resulting path will read `/home/transfer/my-data/`

If a path is not provided with the command the transfers will be written into the folder as set by the configuration property `data_transfers_folder` of the gateway as follows:
```
[grpc.services.gateway]
data_transfers_folder = "/home/MyTransfers"
```
Note that at least one of each must be provided but that the `path` command flag overrides the configuration setting (ie. per transfer).

## 4.1 Do over a transfer
In case the transfer has failed and it is not a driver (rclone) issue, or maybe you want to transfer to another folder, use these 2 steps:
<br>First update the share to `pending`:
```
ocm-share-get-received -state pending 79a2bf32-4bba-437a-ad8f-ec93211375b5
OK
```
Next accept the transfer, optionally with a different path:
```
>> ocm-share-update-received -state accepted -path /home/transfers-sec 79a2bf32-4bba-437a-ad8f-ec93211375b5
OK
```
Now the data will be transferred to the `/home/transfers-sec/my-data/` folder.

Whenever transfer shares are accepted corresponding transfer jobs will be created for them. These can be [managed](#5-managing-transfer-jobs).

## 5. Managing transfer jobs
The transfer driver creates a transfer job for each transfer. These jobs can be managed (request status, retried, cancelled). For this one must first discover the transfer id from the transfers list.

## 5.1 List transfers
List the transfers using the `transfer-list` command to discover their corresponding transfer id:

```
>> transfer-list
+--------------------------------------+--------------------------------------+
| SHAREID.OPAQUEID                     | ID.OPAQUEID                          |
+--------------------------------------+--------------------------------------+
| 2c55dc61-4a06-4f44-9478-78eb1243971b | 0f901f2c-a004-4126-b810-29bf51909035 |
| 1f5de8f0-5565-4694-8eca-f66e578783c8 | f0b3b410-0e39-4591-92f7-8e229650b3c7 |
| 79a2bf32-4bba-437a-ad8f-ec93211375b5 | fe671ae3-0fbf-4b06-b7df-32418c2ebfcb |
+--------------------------------------+--------------------------------------+

```
## 5.2 Show status transfer
Show the current status of a transfer using the `transfer-status` command. Possible transfer states are: 
```
cancelled
cancel failed
complete
expired
failed
in progress
new
invalid
```
```
transfer-get-status -txId fe671ae3-0fbf-4b06-b7df-32418c2ebfcb
+--------------------------------------+--------------------------------------+--------------------------+-----------------------------------+
| SHAREID.OPAQUEID                     | ID.OPAQUEID                          |                   STATUS | CTIME                             |
+--------------------------------------+--------------------------------------+--------------------------+-----------------------------------+
| 79a2bf32-4bba-437a-ad8f-ec93211375b5 | fe671ae3-0fbf-4b06-b7df-32418c2ebfcb | STATUS_TRANSFER_COMPLETE | Mon May 8 12:38:08 +0000 UTC 2023 |
+--------------------------------------+--------------------------------------+--------------------------+-----------------------------------+
```


## 5.5 Retry transfer
Retry a transfer using the `transfer-retry` command with the transfer id specified. This should restart the transfer job and return the new status of the transfer:

```
transfer-retry -txId fe671ae3-0fbf-4b06-b7df-32418c2ebfcb
+--------------------------------------+--------------------------------------+---------------------+-------------------------------+
| SHAREID.OPAQUEID                     | ID.OPAQUEID                          |              STATUS | CTIME                         |
+--------------------------------------+--------------------------------------+---------------------+-------------------------------+
| 79a2bf32-4bba-437a-ad8f-ec93211375b5 | fe671ae3-0fbf-4b06-b7df-32418c2ebfcb | STATUS_TRANSFER_NEW | 2023-05-08 12:41:07 +0000 UTC |
+--------------------------------------+--------------------------------------+---------------------+-------------------------------+
```
## 5.4 Cancel transfer
A running transfer (transfer state `in progress`) can be cancelled using the `transfer-cancel` command as follows:
```
transfer-retry -txId fe671ae3-0fbf-4b06-b7df-32418c2ebfcb
+--------------------------------------+--------------------------------------+---------------------+-------------------------------+
| SHAREID.OPAQUEID                     | ID.OPAQUEID                          |              STATUS | CTIME                         |
+--------------------------------------+--------------------------------------+---------------------+-------------------------------+
| 79a2bf32-4bba-437a-ad8f-ec93211375b5 | fe671ae3-0fbf-4b06-b7df-32418c2ebfcb | STATUS_TRANSFER_CANCELLED | 2023-05-08 13:50:12 +0000 UTC |
+--------------------------------------+--------------------------------------+---------------------+-------------------------------+
```

## 6 Cleanup transfers
Transfers will be removed from the db using the `transfer-cancel` command when the configuration property `remove_transfer_on_cancel` and `remove_transfer_job_on_cancel` of the datatx service and rclone driver respectively have been set to `true` as follows:
```
[grpc.services.datatx]
remove_transfer_on_cancel = true

[grpc.services.datatx.txdrivers.rclone]
remove_transfer_job_on_cancel = true
```
Currently this setting is recommended.

*Note that with these settings `transfer-cancel` will remove transfers & jobs even when a transfer cannot actually be cancelled because it was already in an end-state, eg. `finished` or `failed`. So `transfer-cancel` will act like a 'delete' function.