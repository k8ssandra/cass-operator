// Copyright DataStax, Inc.
// Please see the included license file for details.

//+build mage

package main

import (
	"github.com/magefile/mage/mg"

	// mage:import jenkins
	"github.com/k8ssandra/cass-operator/mage/jenkins"
	// mage:import lint
	_ "github.com/k8ssandra/cass-operator/mage/linting"
)

// Clean all build artifacts, does not clean up old docker images.
func Clean() {
	mg.Deps(operator.Clean)
	mg.Deps(jenkins.Clean)
}
