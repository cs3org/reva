Enhancement: cephmount allow byte ranges

When calling download on the Cephmount driver we previously
did not allow byte ranges, now this is allowed and should be served
since clients like WebDav rclone require this.

https://github.com/cs3org/reva/pull/6034
