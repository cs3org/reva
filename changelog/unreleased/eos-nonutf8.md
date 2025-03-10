Bugfix: stop sending non-UTF8 strings over gRPC

EOS supports having non-UTF8 attributes, which get returned in a Stat. This is problematic for us, as we pass these attributes in the ArbitraryMetadata, which gets sent over gRPC. However, the protobuf language specification states:

>   A string must always contain UTF-8 encoded or 7-bit ASCII text, and cannot be longer than 2^32.

An example of such an attribute is:

user.$KERNEL.PURGE.SEC.FILEHASH="S��ϫ]���z��#1}��uU�v��8�L0R�9j�j��e?�2K�T<sJ�*�l���Dǭ��_[�>η�...��w�w[��Yg"

We fix this by stripping non-UTF8 metadata entries before sending the ResourceInfo over gRPC

https://github.com/cs3org/reva/pull/5119