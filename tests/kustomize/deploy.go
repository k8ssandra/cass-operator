package kustomize

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

func Deploy(namespace string) error {
	return DeployDir(namespace, "kustomize")
}

func Undeploy(namespace string) error {
	return UndeployDir(namespace, "kustomize")
}

func DeployDir(namespace, testDir string) error {
	if err := runMake(namespace, "deploy-test", testDir); err != nil {
		return err
	}
	if kubectl.DockerCredentialsDefined() {
		return createDockerRegistrySecret(namespace)
	}
	return nil
}

func UndeployDir(namespace, testDir string) error {
	return runMake(namespace, "undeploy-test", testDir)
}

func createDockerRegistrySecret(namespace string) error {
	server := os.Getenv(kubectl.EnvDockerServer)
	username := os.Getenv(kubectl.EnvDockerUsername)
	password := os.Getenv(kubectl.EnvDockerPassword)

	args := []string{
		"create", "secret", "docker-registry", "cass-operator-pull-secret",
		"--docker-server=" + server,
		"--docker-username=" + username,
		"--docker-password=" + password,
		"--namespace=" + namespace,
	}
	cmd := exec.Command("kubectl", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		fmt.Printf("createDockerRegistrySecret error output:\n%s\n", out.String())
		return err
	}
	return nil
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
