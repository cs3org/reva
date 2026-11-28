Bugfix: revoke the storage grant of a rejected received share

The gateway committed the grant of a share when the share was created, but never
touched it when the recipient accepted or rejected the share. A recipient who
rejected a share therefore kept the read access its grant carried, and a
recipient who accepted a share they had rejected earlier stayed without access.

Updating a received share now revokes the grant on a rejection and applies it
again on an acceptance. Both calls change the ACL of the owner's resource, and a
storage provider only lets the owner change it, so the gateway makes them as the
owner of the share by minting a token for it, instead of using the recipient's
credentials, which a provider such as EOS refuses.

The grant is revoked before the share provider records the rejection and applied
after it records the acceptance, so a failure leaves the share accepted rather
than rejected but still reachable, and the earlier state is restored when the
grant cannot be committed.

https://github.com/cs3org/reva/pull/6067
