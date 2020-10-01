Enhancement: Add Blackbox Exporter support to Mentix

This update extends Mentix to export a Prometheus SD file specific to the Blackbox Exporter which will be used for initial health monitoring. Usually, Prometheus requires its targets to only consist of the target's hostname; the BBE though expects a full URL here. This makes exporting two distinct files necessary.

https://github.com/cs3org/reva/pull/1190
