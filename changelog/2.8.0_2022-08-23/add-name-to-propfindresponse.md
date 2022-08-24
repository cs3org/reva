Bugfix: Add name to the propfind response

Previously the file- or foldername had to be extracted from the href. This is not nice and
doesn't work for alias links. 

https://github.com/cs3org/reva/pull/3158
