//go:build !race

package layer7

// raceDetectorActive is false in normal (non-race) builds.
// Performance thresholds use their full strict values.
const raceDetectorActive = false
