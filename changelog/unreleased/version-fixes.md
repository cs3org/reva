Bugfix: Fix version number in status page

We needed to undo the version number changes on the status page to keep compatibility for legacy clients. We added a new field `productversion` for the actual version of the product.

https://github.com/cs3org/reva/pull/2876
