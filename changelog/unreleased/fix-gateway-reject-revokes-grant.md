Bugfix: revoke the storage grant of a rejected received share

The gateway committed the grant of a share when the share was created, but never
touched it when the recipient accepted or rejected the share. A recipient who
rejected a share therefore kept the read access its grant carried, and a
recipient who accepted a share they had rejected earlier stayed without access.

Updating a received share now reconciles the grant with the state the share
provider stores: a rejection revokes the grant, an acceptance applies it again.
The state is read back after the update, so an update whose mask leaves the state
alone does not touch the grant either, and updates of the same share are
serialised in the gateway so the two cannot interleave.

The grant changes reuse the hierarchy-aware helpers of the share flows: revoking
the grant of a rejected share re-applies the grants of the shares around it, and
accepting one re-applies the grants of the shares below it that keep higher
permissions. Without that, a rejected parent share would take the grant of an
accepted child share of the same grantee with it on drivers that remove grants
recursively, and an accepted parent would downgrade it.

Both calls change the ACL of the owner's resource, and a storage provider only
lets the owner change it, so the gateway makes them as the owner of the share by
minting a token for it, instead of using the recipient's credentials, which a
provider such as EOS refuses.

A state change whose grant cannot be changed is rolled back, so a share never
stays rejected while its grant is still active, nor accepted without the access
the grant carries.

https://github.com/cs3org/reva/pull/6067
