Bugfix: spaces + apps broken

There were still some places where Reva assumed that we are running spaces; while not verifying this. This caused non-spaces WebUIs to break. This is fixed now.

https://github.com/cs3org/reva/pull/5151