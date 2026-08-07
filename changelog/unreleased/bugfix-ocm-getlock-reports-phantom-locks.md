Bugfix: Report OCM received share locks correctly

`GetLock` on a received OCM share reported every file as locked, with the lock id
`<nil>`. It delegated to gowebdav's `GetLock`, which stats with a fixed property
set that omits `lockdiscovery`, and gowebdav renders a missing property as the
string `<nil>` rather than an empty one, so no emptiness check could tell an
absent lock from a present one. The lock state is now read from an explicit
`lockdiscovery` PROPFIND and parsed from the `activelock` element, and a resource
with no lock reports as unlocked.

https://github.com/owncloud/reva/pull/698
