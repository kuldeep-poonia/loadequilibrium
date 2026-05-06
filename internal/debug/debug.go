package debug

import "sync/atomic"

var hotPathLogs uint32 = 0

// EnableHotPathLogs enables or disables high-frequency hot path logging.
// Default: disabled.
func EnableHotPathLogs(enable bool) {
	if enable {
		atomic.StoreUint32(&hotPathLogs, 1)
	} else {
		atomic.StoreUint32(&hotPathLogs, 0)
	}
}

// HotPathLogsEnabled returns true if hot path debug logging is enabled.
func HotPathLogsEnabled() bool {
	return atomic.LoadUint32(&hotPathLogs) == 1
}