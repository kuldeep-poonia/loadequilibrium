// Package control implements the single executable decision authority.
//
// Autopilot, intelligence, policy, sandbox, and optimisation modules feed this
// package as advisory signal generators. They may shape constraints, costs, and
// candidate sets, but only the control authority emits executable directives.
//
// Runtime integration point: phaseRuntime.apply() in runtime/phase_runtime.go.
package control
