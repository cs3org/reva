Changelog for reva 2.10.0 (2022-09-09)
=======================================

The following sections list the changes in reva 2.10.0 relevant to
reva users. The changes are ordered by importance.

Summary
-------

*   Fix #3210: Jsoncs3 mtime fix
*   Enh #3213: Allow for dumping the public shares from the cs3 publicshare manager
*   Enh #3199: Add support for cs3 storage backends to the json publicshare manager

Details
-------

*   Bugfix #3210: Jsoncs3 mtime fix

   We now correctly update the mtime to only sync when the file changed on the storage.

   https://github.com/cs3org/reva/pull/3210

*   Enhancement #3213: Allow for dumping the public shares from the cs3 publicshare manager

   We enhanced the cs3 publicshare manager to support dumping its content during a publicshare
   manager migration.

   https://github.com/cs3org/reva/pull/3213

*   Enhancement #3199: Add support for cs3 storage backends to the json publicshare manager

   We enhanced the json publicshare manager to support a cs3 storage backend alongside the file
   and memory backends.

   https://github.com/cs3org/reva/pull/3199
