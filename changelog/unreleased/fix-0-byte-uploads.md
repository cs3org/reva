Bugfix: Fix 0-byte-uploads

We fixed a problem with 0-byte uploads by using TouchFile instead of going through TUS (decomposedfs and owncloudsql storage drivers only for now).

https://github.com/cs3org/reva/pull/2998
