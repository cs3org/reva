Enhancement: wopi driver: force preview mode for external or non-owner users

This patch uses the preview mode as opposed to read-write mode when presenting
an app to a user that is either anonymous (public links) or sharee (thus not the owner).
This prevents the file from being touched/locked right when opened.

https://github.com/cs3org/reva/pull/3768
