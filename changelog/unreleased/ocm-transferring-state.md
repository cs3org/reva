Enhancement: Utilize the new transferring state in the cs3api

- Puts the ocm share in a transferring state when doing the processing of an embedded share is ongoing
- A callback is implemented that puts the share in the accepted state when the transfer is finished

https://github.com/cs3org/reva/pull/5643
