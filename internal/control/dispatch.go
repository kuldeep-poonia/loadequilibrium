package control

import "github.com/loadequilibrium/loadequilibrium/internal/optimisation"

type DirectiveDispatcher interface {
	Dispatch(tickIndex uint64, directives map[string]optimisation.ControlDirective)
}

// Dispatch is the single runtime handoff from control authority output to the
// actuator subsystem.
func Dispatch(act DirectiveDispatcher, tickIndex uint64, directives map[string]optimisation.ControlDirective) {
	if act == nil {
		return
	}
	act.Dispatch(tickIndex, directives)
}
