Bugfix: Fix view&download permission issue

When opening files with view&download permission (aka read), the appprovider would falsely issue a secureview token. This is fixed now.

https://github.com/cs3org/reva/pull/5055
