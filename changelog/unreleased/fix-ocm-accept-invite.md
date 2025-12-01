Bugfix: Set correct user type when accepting OCM invites

When a remote user accepts an OCM invite, they were being stored with
USER_TYPE_PRIMARY instead of USER_TYPE_FEDERATED. This caused federated
user searches to fail and OCM share creation to break because the user
ID was not properly formatted with the @domain suffix required for OCM
address resolution.

https://github.com/cs3org/reva/pull/5415
