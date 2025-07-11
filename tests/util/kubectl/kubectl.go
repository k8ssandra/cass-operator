// Copyright DataStax, Inc.
// Please see the included license file for details.

package kubectl

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/errors"

	"golang.org/x/term"

	shutil "github.com/k8ssandra/cass-operator/tests/util/sh"
)

const (
	// Credentials for creating an ImagePullSecret
	EnvDockerUsername = "M_DOCKER_USERNAME"
	EnvDockerPassword = "M_DOCKER_PASSWORD"
	EnvDockerServer   = "M_DOCKER_SERVER"
)

func WatchPods() {
	args := []string{"pods", "-w"}
	kCmd := KCmd{Command: "get", Args: args, Flags: nil}
	kCmd.ExecVPanic()
}

func WatchPodsInNs(namespace string) {
	args := []string{"pods", "--namespace", namespace, "-w"}
	kCmd := KCmd{Command: "watch", Args: args, Flags: nil}
	kCmd.ExecVPanic()
}

// ==============================================
// KCmd represents an executable kubectl command
// ==============================================
type KCmd struct {
	Command string
	Args    []string
	Flags   map[string]string
}

// ==============================================
// Execute KCmd by running kubectl
// ==============================================
func (k KCmd) ToCliArgs() []string {
	var args []string
	// Write out flags first because we don't know
	// if the command args will have a -- in them or not
	// and prevent our flags from working.
	for k, v := range k.Flags {
		args = append(args, fmt.Sprintf("--%s=%s", k, v))
	}
	args = append(args, k.Command)
	args = append(args, k.Args...)
	return args
}

// ExecV runs KCmd via `kubectl`, where KCmd is a struct holding the kubectl command to run (not including `kubectl` itself), the args, and any flags.
// Returns (stdout, stderr, error) and also logs logs output.
func (k KCmd) ExecVCapture() (string, string, error) {
	return shutil.RunVCapture("kubectl", k.ToCliArgs()...)
}

// ExecV runs KCmd via `kubectl`, where KCmd is a struct holding the kubectl command to run (not including `kubectl` itself), the args, and any flags.
// Returns error only (no capture of results) and also logs logs output.
func (k KCmd) ExecV() error {
	return shutil.RunV("kubectl", k.ToCliArgs()...)
}

func (k KCmd) ExecVPanic() {
	shutil.RunVPanic("kubectl", k.ToCliArgs()...)
}

func (k KCmd) Output() (string, error) {
	return shutil.Output("kubectl", k.ToCliArgs()...)
}

func (k KCmd) OutputPanic() string {
	return shutil.OutputPanic("kubectl", k.ToCliArgs()...)
}

// ==============================================
// Helper functions to build up a KCmd object
// for common actions
// ==============================================
func (k KCmd) InNamespace(namespace string) KCmd {
	return k.WithFlag("namespace", namespace)
}

func (k KCmd) FormatOutput(outputType string) KCmd {
	return k.WithFlag("output", outputType)
}

func (k KCmd) WithFlag(name string, value string) KCmd {
	if k.Flags == nil {
		k.Flags = make(map[string]string)
	}
	k.Flags[name] = value
	return k
}

func (k KCmd) WithLabel(label string) KCmd {
	k.Args = append(k.Args, "-l", label)
	return k
}

func ClusterInfoForContext(ctxt string) KCmd {
	args := []string{"--context", ctxt}
	return KCmd{Command: "cluster-info", Args: args}
}

func CreateNamespace(namespace string) KCmd {
	args := []string{"namespace", namespace}
	return KCmd{Command: "create", Args: args}
}

func DeleteNamespace(namespace string) KCmd {
	args := []string{"namespace", namespace}
	return KCmd{Command: "delete", Args: args}
}

func CreateSecretLiteral(name string, user string, pw string) KCmd {
	args := []string{"secret", "generic", name}
	flags := map[string]string{
		"from-literal=username": user,
		"from-literal=password": pw,
	}
	return KCmd{Command: "create", Args: args, Flags: flags}
}

func Label(nodes string, key string, value string) KCmd {
	tokens := strings.Split(nodes, " ")
	args := []string{}
	for _, t := range tokens {
		if t != "" {
			args = append(args, "nodes/"+t)
		}
	}
	label := fmt.Sprintf("%s=%s", key, value)
	args = append(args, label)
	args = append(args, "--overwrite")
	return KCmd{Command: "label", Args: args}
}

func Taint(node string, key string, value string, effect string) KCmd {
	var args []string
	if value != "" {
		args = []string{"nodes", node, fmt.Sprintf("%s=%s:%s", key, value, effect)}
	} else {
		args = []string{"nodes", node, fmt.Sprintf("%s:%s", key, effect)}
	}

	return KCmd{Command: "taint", Args: args}
}

func Annotate(resource string, name string, key string, value string) KCmd {
	args := []string{resource, name, fmt.Sprintf("%s=%s", key, value)}
	return KCmd{Command: "annotate", Args: args}
}

func CreateFromFiles(paths ...string) KCmd {
	var args []string
	for _, p := range paths {
		args = append(args, "-f", p)
	}
	return KCmd{Command: "create", Args: args}
}

func Logs(args ...string) KCmd {
	return KCmd{Command: "logs", Args: args}
}

func Get(args ...string) KCmd {
	return KCmd{Command: "get", Args: args}
}

func Describe(args ...string) KCmd {
	return KCmd{Command: "describe", Args: args}
}

func GetByTypeAndName(resourceType string, names ...string) KCmd {
	var args []string
	for _, n := range names {
		args = append(args, fmt.Sprintf("%s/%s", resourceType, n))
	}
	return KCmd{Command: "get", Args: args}
}

func GetByFiles(paths ...string) KCmd {
	var args []string
	for _, path := range paths {
		args = append(args, "-f", path)
	}
	return KCmd{Command: "get", Args: args}
}

func DeleteFromFiles(paths ...string) KCmd {
	var args []string
	for _, path := range paths {
		args = append(args, "-f", path)
	}
	return KCmd{Command: "delete", Args: args}
}

func Delete(args ...string) KCmd {
	return KCmd{Command: "delete", Args: args}
}

func DeleteByTypeAndName(resourceType string, names ...string) KCmd {
	var args []string
	for _, n := range names {
		args = append(args, fmt.Sprintf("%s/%s", resourceType, n))
	}
	return KCmd{Command: "delete", Args: args}
}

func ApplyFiles(paths ...string) KCmd {
	var args []string
	for _, path := range paths {
		args = append(args, "-f", path)
	}
	return KCmd{Command: "apply", Args: args}
}

func PatchMerge(resource string, data string) KCmd {
	args := []string{resource, "--patch", data, "--type", "merge"}
	return KCmd{Command: "patch", Args: args}
}

func PatchJson(resource string, data string) KCmd {
	args := []string{resource, "--patch", data, "--type", "json"}
	return KCmd{Command: "patch", Args: args}
}

func Patch(resource string, data string) KCmd {
	args := []string{resource, "-p", data}
	return KCmd{Command: "patch", Args: args}
}

func erasePreviousLine() {
	// cursor up one line
	fmt.Print("\033[A")

	// erase line
	fmt.Print("\033[K")
}

func waitForOutputPattern(k KCmd, pattern string, seconds int) error {
	re := regexp.MustCompile(pattern)
	c := make(chan string)
	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	cquit := make(chan bool)
	defer close(cquit)

	counter := 0
	outputIsTerminal := term.IsTerminal(int(os.Stdout.Fd()))
	var actual string
	var err error

	go func() {
		printedRerunMsg := false
		for !re.MatchString(actual) {
			select {
			case <-cquit:
				fmt.Println("")
				return
			default:
				actual, err = k.Output()
				if outputIsTerminal && counter > 0 {
					erasePreviousLine()

					if printedRerunMsg {
						// We need to erase both the new exec output,
						// as well as our previous "rerunning" line now
						erasePreviousLine()
					}

					fmt.Printf("Rerunning previous command (%v)\n", counter)
					printedRerunMsg = true
				}
				counter++
				// Execute at most once every two seconds
				time.Sleep(time.Second * 2)
			}
		}
		c <- actual
	}()

	select {
	case <-timer.C:
		var expectedPhrase string
		expectedPhrase = "Expected output to match regex: "
		msg := fmt.Sprintf("Timed out waiting for value. %s '%s', but '%s' did not match", expectedPhrase, pattern, actual)
		if err != nil {
			msg = fmt.Sprintf("%s\nThe following error occurred while querying k8s: %v", msg, err)
		}
		e := errors.New(msg)
		return e
	case <-c:
		return nil
	}
}

func WaitForOutputPattern(k KCmd, pattern string, seconds int) error {
	return waitForOutputPattern(k, pattern, seconds)
}

func WaitForOutput(k KCmd, expected string, seconds int) error {
	return waitForOutputPattern(k, fmt.Sprintf("^%s$", regexp.QuoteMeta(expected)), seconds)
}

func WaitForOutputContains(k KCmd, expected string, seconds int) error {
	return waitForOutputPattern(k, regexp.QuoteMeta(expected), seconds)
}

func DumpAllLogs(path string) KCmd {
	// Make dir if doesn't exist
	_ = os.MkdirAll(path, os.ModePerm)
	args := []string{"dump", "-A"}
	flags := map[string]string{"output-directory": path}
	return KCmd{Command: "cluster-info", Args: args, Flags: flags}
}

func DumpLogs(path string, namespace string) KCmd {
	// Make dir if doesn't exist
	_ = os.MkdirAll(path, os.ModePerm)
	args := []string{"dump", "-n", namespace}
	flags := map[string]string{"output-directory": path}
	return KCmd{Command: "cluster-info", Args: args, Flags: flags}
}

// DumpClusterInfo Executes `kubectl cluster-info dump -o yaml` on each cluster. The output
// is stored under <project-root>/build/test.
func DumpClusterInfo(path string, namespace string) {
	_ = os.MkdirAll(path, os.ModePerm)

	dumpCmd := DumpLogs(path, namespace)
	dumpCmd.ExecVPanic()

	// Dump all objects that we need to investigate failures as a flat list and as yaml manifests
	for _, objectType := range []string{"statefulsets", "pvc", "pv", "pods", "CassandraDatacenter", "CassandraTask"} {
		// Get the list of objects
		output, _ := Get(objectType, "-o", "wide", "-n", namespace).Output()
		storeOutput(path, objectType, "out", output)

		// Get the yamls for each object
		output, _ = Get(objectType, "-o", "yaml", "-n", namespace).Output()
		storeOutput(path, objectType, "yaml", output)
	}

	// For describe information
	for _, objectType := range []string{"statefulsets", "pods", "pvc", "pv"} {
		output, _ := Describe(objectType, "-n", namespace).Output()
		storeOutput(path, objectType, "out", output)
	}
}

func storeOutput(path, objectType, ext, output string) {
	filePath := fmt.Sprintf("%s/%s.%s", path, objectType, ext)
	outputFile, err := os.Create(filePath)
	if err != nil {
		panic("Failed to create log file")
	}
	defer func() {
		if err := outputFile.Close(); err != nil {
			panic(fmt.Sprintf("Failed to close output file: %v", err))
		}
	}()
	_, err = outputFile.WriteString(output)
	if err != nil {
		panic(err)
	}
	err = outputFile.Sync()
	if err != nil {
		panic(err)
	}
}

func ExecOnPod(podName string, args ...string) KCmd {
	execArgs := []string{podName}
	execArgs = append(execArgs, args...)
	return KCmd{Command: "exec", Args: execArgs}
}

func GetNodes() KCmd {
	json := "jsonpath={range .items[*]}{@.metadata.name} {end}"
	args := []string{"nodes", "-o", json}
	return KCmd{Command: "get", Args: args}
}

func GetNodeNameForPod(podName string) KCmd {
	json := "jsonpath={.spec.nodeName}"
	return Get(fmt.Sprintf("pod/%s", podName)).FormatOutput(json)
}

func DockerCredentialsDefined() bool {
	_, ok := os.LookupEnv(EnvDockerUsername)
	return ok
}
