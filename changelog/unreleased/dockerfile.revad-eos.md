Enhancement: Bring back multi-stage build to save on image size

  - Use EOS 4.8.91 as base image
  - Bring back multi-stage build
  - Build revad on the eos 4.8.91 image due to missing dependency (`ld-musl-x86_64.so.1`, typical of alpine)
  - Copy the resulting revad from the builder container

Resulting image size (unpacked on disk) is 2.59GB
  - eos-all:4.8.91 is 2.47GB
  - existing revad:latest-eos is 6.18GB

https://github.com/cs3org/reva/pull/3197
