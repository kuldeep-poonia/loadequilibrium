package config

import "runtime"

// cpuCount returns the number of logical CPUs available to the process.
// Used to set a sensible default for WORKER_POOL_SIZE without importing
// "runtime" in every caller.
func cpuCount() int {
	n := runtime.NumCPU()
	if n < 4 {
		return 4 // floor: at least 4 workers even on 1-2 core containers
	}
	if n > 64 {
		return 64 // ceiling: avoids over-subscription on very large machines
	}
	return n
}
