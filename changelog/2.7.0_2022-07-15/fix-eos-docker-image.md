Bugfix: Fix revad with EOS docker image

We've fixed the revad with EOS docker image. Previously the revad
binary was build on Alpine and not executable on the final RHEL based image.

https://github.com/cs3org/reva/issues/3036
