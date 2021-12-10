Bugfix: If a trace is not available do not log default trace value

Prevent from logging `traceid=0000000000000000` if there is no traceid for the given span.

https://github.com/cs3org/reva/pull/2352