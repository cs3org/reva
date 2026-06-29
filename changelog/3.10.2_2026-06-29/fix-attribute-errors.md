Bugfix: Fix for cephmount lock & xattr bugs

* ReadArbitraryMetadata - Returns stored user.* xattrs in GetMD instead of nothing.
* Paths Unlock/SetLock open the full chroot path instead of the incorrect relative path.
* Quality of life improvements for error handling & debug logs.

https://github.com/cs3org/reva/pull/5650