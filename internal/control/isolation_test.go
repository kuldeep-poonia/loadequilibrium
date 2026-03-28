package control_test

import (
	"go/build"
	"testing"
)

// TestControlPackageIsIsolated verifies that no runtime package
// imports internal/control. This enforces the architectural boundary
// until an explicit integration decision is made.
func TestControlPackageIsIsolated(t *testing.T) {
	runtimePkgs := []string{
		"github.com/loadequilibrium/loadequilibrium/internal/runtime",
		"github.com/loadequilibrium/loadequilibrium/internal/autopilot",
		"github.com/loadequilibrium/loadequilibrium/internal/intelligence",
	}
	controlPkg := "github.com/loadequilibrium/loadequilibrium/internal/control"
	for _, pkg := range runtimePkgs {
		p, err := build.Import(pkg, "", 0)
		if err != nil {
			t.Skipf("package %s not resolvable: %v", pkg, err)
		}
		for _, imp := range p.Imports {
			if imp == controlPkg {
				t.Errorf("ISOLATION BREACH: %s imports %s", pkg, controlPkg)
			}
		}
	}
}
