Bugfix: Make dataproviders return more headers

Instead of ocdav doing an additional Stat request we now rely on the dataprovider to return the necessary metadata information as headers.

https://github.com/owncloud/reva/issues/3080