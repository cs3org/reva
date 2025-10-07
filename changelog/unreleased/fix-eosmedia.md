Bugfix: make eosmedia work with new space structure

We have added a special case to the `spacesLevel` function
to make eosmedia's spaces work. This is temporary, since we
plan to get rid of this function and just use the SpaceID that
comes from the projects database.

https://github.com/cs3org/reva/pull/5347
