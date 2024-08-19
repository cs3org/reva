Bugfix: ocm: fixed content type of the discovery endpoint,

Based on the [OCM specification](https://cs3org.github.io/OCM-API/docs.html?repo=OCM-API&user=cs3org#/paths/~1ocm-provider/get) the discovery endpoint
should return `application/hal+json`.

https://github.com/cs3org/reva/pull/4815
