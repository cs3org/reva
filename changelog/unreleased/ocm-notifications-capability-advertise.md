Enhancement: advertise the OCM notifications capability

Enabled OCM discovery now lists `notifications` in its capabilities, so
peers that gate notification sending on that capability can reach
`POST /notifications`. The advertised apiVersion stays 1.4.0, and a
disabled discovery document still advertises no capabilities.
