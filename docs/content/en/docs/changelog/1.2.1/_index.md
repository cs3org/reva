
---
title: "v1.2.1"
linkTitle: "v1.2.1"
weight: 40
description: >
  Changelog for Reva v1.2.1 (2020-09-15)
---

Changelog for reva 1.2.1 (2020-09-15)
=======================================

The following sections list the changes in reva 1.2.1 relevant to
reva users. The changes are ordered by importance.

Summary
-------

 * Fix #1124: Do not swallow 'not found' errors in Stat
 * Enh #1125: Rewire dav files to the home storage
 * Enh #559: Introduce ocis storage driver
 * Enh #1118: Metrics module can be configured to retrieve metrics data from file

Details
-------

 * Bugfix #1124: Do not swallow 'not found' errors in Stat

   Webdav needs to determine if a file exists to return 204 or 201 response codes. When stating a non
   existing resource the NOT_FOUND code was replaced with an INTERNAL error code. This PR passes
   on a NOT_FOUND status code in the gateway.

   https://github.com/cs3org/reva/pull/1124

 * Enhancement #1125: Rewire dav files to the home storage

   If the user specified in the dav files URL matches the current one, rewire it to use the
   webDavHandler which is wired to the home storage.

   This fixes path mapping issues.

   https://github.com/cs3org/reva/pull/1125

 * Enhancement #559: Introduce ocis storage driver

   We introduced a now storage driver `ocis` that deconstructs a filesystem and uses a node first
   approach to implement an efficient lookup of files by path as well as by file id.

   https://github.com/cs3org/reva/pull/559

 * Enhancement #1118: Metrics module can be configured to retrieve metrics data from file

   - Export site metrics in Prometheus #698

   https://github.com/cs3org/reva/pull/1118


