Bugfix: Temporarily exclude ceph-iscsi when building revad-ceph image

Due to `Package ceph-iscsi-3.6-1.el8.noarch.rpm is not signed` error when
building the revad-ceph docker image, the package `ceph-iscsi` has been excluded from the dnf update.
It will be included again once the pkg will be signed again.

https://github.com/cs3org/reva/pull/4032
