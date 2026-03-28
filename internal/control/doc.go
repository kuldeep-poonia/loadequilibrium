// Package control implements a coordinated stochastic MPC controller
// that was superseded by the intelligence/autopilot layers during
// architectural iteration but not removed.
//
// STATUS: ISOLATED — not imported by any runtime path.
// DO NOT import this package without a full architectural review.
// The CoordinatedOptimizer, RegimeMemory, and JointSimulator here
// are structurally more rigorous than the active system's heuristic
// blend and represent a viable integration path if the merge authority
// issue is resolved.
//
// Audit reference: mergeDirective() in runtime/phase_runtime.go.
package control
