Enhancement: revamp ScienceMesh integration tests

This extends the ScienceMesh tests by running a wopiserver next
to each EFSS/IOP, and by including a CERNBox-like minimal configuration.
The latter is based on local storage and in-memory shares (no db dependency).

https://github.com/cs3org/reva/pull/4246
