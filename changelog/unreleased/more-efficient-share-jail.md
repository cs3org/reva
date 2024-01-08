Bugfix: more efficient share jail

The share jail was stating every shared recource twice when listing the share jail root. For no good reason. And it was not sending filters when it could.

https://github.com/cs3org/reva/pull/4452
