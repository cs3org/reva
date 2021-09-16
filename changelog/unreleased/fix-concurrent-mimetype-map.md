Bugfix: Fix concurrent registration of mimetypes

We fixed registering mimetypes in the mime package when starting multiple storage providers in the same process.

https://github.com/cs3org/reva/pull/2077
