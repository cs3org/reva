package antivirus

import "time"

var timeout = 300 * time.Second
var icapUrl = "icap://127.0.0.1:1344"
var icapService = "avscan"
var clamavSocket = "/tmp/clamd.socket"

type ScanResult struct {
	Infected bool
}
