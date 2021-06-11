package kustomize

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func Deploy(namespace string) error {
	return runMake(namespace, "deploy-test")
}

func Undeploy(namespace string) error {
	return runMake(namespace, "undeploy-test")
}

func runMake(namespace, command string) error {
	ns := fmt.Sprintf("NAMESPACE=%s", namespace)
	deploy := exec.Command("make", ns, command)
	var out bytes.Buffer
	deploy.Stdout = &out

	path, err := os.Getwd()
	if err != nil {
		return err
	}

	makeDir, err := os.Open(filepath.Join(path, "..", ".."))
	if err != nil {
		return err
	}

	deploy.Dir = makeDir.Name()

	fmt.Printf("Running make in: %s\n", makeDir.Name())

	err = deploy.Run()
	if err != nil {
		return err
	}

	return nil
}
