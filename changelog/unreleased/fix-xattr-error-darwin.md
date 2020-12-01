Bugfix: Fix xattrr.Remove error check for macOS

Previously, we checked the xattrr.Remove only for linux systems. Now macOS is checked also 

https://github.com/cs3org/reva/pull/1351