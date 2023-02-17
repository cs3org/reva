Enhancement: Introduce ini file based metadata backend

We added a new metadata backend for the decomposed storage driver that uses an additional `.ini` file to store file metadata. This allows scaling beyond some filesystem specific xattr limitations.

https://github.com/cs3org/reva/pull/3649