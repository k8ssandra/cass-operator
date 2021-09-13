package kustomize

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func Deploy(namespace string) error {
	return DeployDir(namespace, "kustomize")
}

func Undeploy(namespace string) error {
	return UndeployDir(namespace, "kustomize")
}

func DeployDir(namespace, testDir string) error {
	return runMake(namespace, "deploy-test", testDir)
}

func UndeployDir(namespace, testDir string) error {
	return runMake(namespace, "undeploy-test", testDir)
}

func runMake(namespace, command, dir string) error {
	ns := fmt.Sprintf("NAMESPACE=%s", namespace)
	kustDir := fmt.Sprintf("TEST_DIR=%s", dir)
	deploy := exec.Command("make", ns, command, kustDir)
	var out bytes.Buffer
	deploy.Stdout = &out
	deploy.Stderr = &out

	path, err := os.Getwd()
	if err != nil {
		fmt.Printf("Getwd error output:\n%s\n", out.String())
		return err
	}

	makeDir, err := os.Open(filepath.Join(path, "..", ".."))
	if err != nil {
		fmt.Printf("os.Open error output:\n%s\n", out.String())
		return err
	}

	deploy.Dir = makeDir.Name()

	err = deploy.Run()
	if err != nil {
		fmt.Printf("Run error output:\n%s\n", out.String())
		return err
	}

	return nil
}
