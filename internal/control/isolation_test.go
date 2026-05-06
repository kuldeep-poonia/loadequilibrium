package control_test

import (
	"go/build"
	"testing"
)

// TestControlPackageAuthorityBoundary verifies that runtime is the only
// production package allowed to import internal/control directly. Autopilot,
// intelligence, sandbox, and optimisation remain advisory/model packages.
func TestControlPackageAuthorityBoundary(t *testing.T) {
	controlPkg := "github.com/loadequilibrium/loadequilibrium/internal/control"
	runtimePkg := "github.com/loadequilibrium/loadequilibrium/internal/runtime"

	p, err := build.Import(runtimePkg, "", 0)
	if err != nil {
		t.Skipf("package %s not resolvable: %v", runtimePkg, err)
	}
	found := false
	for _, imp := range p.Imports {
		if imp == controlPkg {
			found = true
		}
	}
	if !found {
		t.Fatalf("runtime must import %s as the single decision authority", controlPkg)
	}

	advisoryPkgs := []string{
		"github.com/loadequilibrium/loadequilibrium/internal/autopilot",
		"github.com/loadequilibrium/loadequilibrium/internal/intelligence",
		"github.com/loadequilibrium/loadequilibrium/internal/optimisation",
		"github.com/loadequilibrium/loadequilibrium/internal/sandbox",
	}
	for _, pkg := range advisoryPkgs {
		p, err := build.Import(pkg, "", 0)
		if err != nil {
			t.Skipf("package %s not resolvable: %v", pkg, err)
		}
		for _, imp := range p.Imports {
			if imp == controlPkg {
				t.Errorf("AUTHORITY BREACH: advisory package %s imports %s", pkg, controlPkg)
			}
		}
	}
}
