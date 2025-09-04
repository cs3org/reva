Bugfix: add version directory to propfind response

PROPFINDs to <resource>/v return a list of all versions of a resource. This list should start with a reference to the version directory itself,
which in turn is filtered out by the front-end. This entry was missing after a refactor of the versions to make them spaces-compatible. This change now fixes this.

https://github.com/cs3org/reva/pull/5265