//go:build race

package layer7

// raceDetectorActive is true when the test binary was compiled with -race.
// The race detector adds ~5-10x overhead to every atomic, mutex, and channel
// operation, which significantly reduces orchestrator tick throughput.
// Tests that assert minimum tick counts must account for this.
const raceDetectorActive = true
