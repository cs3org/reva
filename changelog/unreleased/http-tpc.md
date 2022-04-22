Enhancement: Add support for HTTP TPC

We have added support for HTTP Third Party Copy.
This allows remote data transfers between storages managed by either two different reva servers,
or a reva server and a Grid (WLCG/ESCAPE) site server.

Such remote transfers are expected to be driven by [GFAL](https://cern.ch/dmc-docs/gfal2/gfal2.html),
the underlying library used by [FTS](https://cern.ch/fts), and [Rucio](https://rucio.cern.ch).

In addition, the oidcmapping package has been refactored to
support the standard OIDC use cases as well when no mapping
is defined.

https://github.com/cs3org/reva/issues/1787
https://github.com/cs3org/reva/pull/2007
