Bugfix: public links on cephmount

* Public links and OCM shares take their role from the token, as on EOS, instead of requiring a grant xattr that nothing ever wrote.
* The cboxexternal service account now gets rwx wherever an external account goes, on first use, and keeps it. A read-only share no longer narrows its access for other shares and links on the same folder.
* External accounts cannot go through symlinks, and a stat never changes the ACL.

https://github.com/cs3org/reva/pull/XXXX
