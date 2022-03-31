// Copyright DataStax, Inc.
// Please see the included license file for details.

package ginkgo_util

import (
	"encoding/base64"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	ginkgo "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	mageutil "github.com/k8ssandra/cass-operator/tests/util"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

const (
	EnvNoCleanup        = "M_NO_CLEANUP"
	ImagePullSecretName = "imagepullsecret"
)

func duplicate(value string, count int) string {
	result := []string{}
	for i := 0; i < count; i++ {
		result = append(result, value)
	}

	return strings.Join(result, " ")
}

// Wrapper type to make it simpler to
// set a namespace one time and execute all of your
// KCmd objects inside of it, and then use Gomega
// assertions on panic
type NsWrapper struct {
	Namespace     string
	TestSuiteName string
	LogDir        string
	stepCounter   int
}

func NewWrapper(suiteName string, namespace string) NsWrapper {
	return NsWrapper{
		Namespace:     namespace,
		TestSuiteName: suiteName,
		LogDir:        genSuiteLogDir(suiteName),
		stepCounter:   1,
	}
}

// ExecVCapture runs KCmd via `kubectl` in the namspace (the receiver), where KCmd is a struct holding the kubectl command to run (not including `kubectl` itself), the args, and any flags.
// Returns (stdout, stderr, error) and also logs logs output.
func (k NsWrapper) ExecVCapture(kcmd kubectl.KCmd) (string, string, error) {
	return kcmd.InNamespace(k.Namespace).ExecVCapture()
}

// ExecV runs KCmd via `kubectl` in the namspace (the receiver), where KCmd is a struct holding the kubectl command to run (not including `kubectl` itself), the args, and any flags.
// Returns error only (no capture of results) and also logs logs output.
func (k NsWrapper) ExecV(kcmd kubectl.KCmd) error {
	err := kcmd.InNamespace(k.Namespace).ExecV()
	return err
}

func (k NsWrapper) ExecVPanic(kcmd kubectl.KCmd) {
	err := kcmd.InNamespace(k.Namespace).ExecV()
	Expect(err).ToNot(HaveOccurred())
}

func (k NsWrapper) Output(kcmd kubectl.KCmd) (string, error) {
	out, err := kcmd.InNamespace(k.Namespace).Output()
	return out, err
}

func (k NsWrapper) OutputPanic(kcmd kubectl.KCmd) string {
	out, err := kcmd.InNamespace(k.Namespace).Output()
	Expect(err).ToNot(HaveOccurred())
	return out
}

func (k NsWrapper) WaitForOutput(kcmd kubectl.KCmd, expected string, seconds int) error {
	return kubectl.WaitForOutput(kcmd.InNamespace(k.Namespace), expected, seconds)
}

func (k NsWrapper) WaitForOutputContains(kcmd kubectl.KCmd, expected string, seconds int) error {
	return kubectl.WaitForOutputContains(kcmd.InNamespace(k.Namespace), expected, seconds)
}

func (k NsWrapper) WaitForOutputPanic(kcmd kubectl.KCmd, expected string, seconds int) {
	err := kubectl.WaitForOutput(kcmd.InNamespace(k.Namespace), expected, seconds)
	Expect(err).ToNot(HaveOccurred())
}

func (k NsWrapper) WaitForOutputContainsPanic(kcmd kubectl.KCmd, expected string, seconds int) {
	err := kubectl.WaitForOutput(kcmd.InNamespace(k.Namespace), expected, seconds)
	Expect(err).ToNot(HaveOccurred())
}

func (k NsWrapper) WaitForOutputPattern(kcmd kubectl.KCmd, pattern string, seconds int) error {
	return kubectl.WaitForOutputPattern(kcmd.InNamespace(k.Namespace), pattern, seconds)
}

func (k *NsWrapper) countStep() int {
	n := k.stepCounter
	k.stepCounter++
	return n
}

func (ns NsWrapper) Terminate() {
	noCleanup := os.Getenv(EnvNoCleanup)
	if strings.ToLower(noCleanup) == "true" {
		fmt.Println("Skipping namespace cleanup and deletion.")
		return
	}

	fmt.Println("Cleaning up and deleting namespace.")
	// Always try to delete the dc that was used in the test
	// incase the test failed out before a delete step.
	//
	// This is important because deleting the namespace itself
	// can hang if this step is skipped.
	kcmd := kubectl.Delete("cassandradatacenter", "--all")
	_, _, dcErr := ns.ExecVCapture(kcmd)
	Expect(dcErr).ToNot(HaveOccurred())
}

//===================================
// Logging functions for the NsWrapper
// that execute the Kcmd and then dump
// k8s logs for that namespace
//====================================
func sanitizeForLogDirs(s string) string {
	reg, err := regexp.Compile(`[\s\\\/\-\.,]`)
	mageutil.PanicOnError(err)
	return reg.ReplaceAllLiteralString(s, "_")
}

func genSuiteLogDir(suiteName string) string {
	datetime := time.Now().Format("2006-01-02_15-04-05")
	return fmt.Sprintf("../../build/kubectl_dump/%s/%s",
		sanitizeForLogDirs(suiteName), datetime)
}

func (ns *NsWrapper) genTestLogDir(description string) string {
	sanitizedDesc := sanitizeForLogDirs(description)
	return fmt.Sprintf("%s/%02d_%s", ns.LogDir, ns.countStep(), sanitizedDesc)
}

func (ns *NsWrapper) ExecAndLog(description string, kcmd kubectl.KCmd) {
	ginkgo.By(description)
	defer kubectl.DumpLogs(ns.genTestLogDir(description), ns.Namespace).ExecVPanic()
	execErr := ns.ExecV(kcmd)
	Expect(execErr).ToNot(HaveOccurred())
}

func (ns *NsWrapper) ExecAndLogAndExpectErrorString(description string, kcmd kubectl.KCmd, expectedError string) {
	ginkgo.By(description)
	defer kubectl.DumpLogs(ns.genTestLogDir(description), ns.Namespace).ExecVPanic()
	_, captureErr, execErr := ns.ExecVCapture(kcmd)
	Expect(execErr).To(HaveOccurred())
	Expect(captureErr).Should(ContainSubstring(expectedError))
}

func (ns *NsWrapper) OutputAndLog(description string, kcmd kubectl.KCmd) string {
	ginkgo.By(description)
	defer kubectl.DumpLogs(ns.genTestLogDir(description), ns.Namespace).ExecVPanic()
	output, execErr := ns.Output(kcmd)
	Expect(execErr).ToNot(HaveOccurred())
	return output
}

func (ns *NsWrapper) WaitForOutputAndLog(description string, kcmd kubectl.KCmd, expected string, seconds int) {
	ginkgo.By(description)
	defer kubectl.DumpLogs(ns.genTestLogDir(description), ns.Namespace).ExecVPanic()
	execErr := ns.WaitForOutput(kcmd, expected, seconds)
	Expect(execErr).ToNot(HaveOccurred())
}

func (ns *NsWrapper) WaitForOutputPatternAndLog(description string, kcmd kubectl.KCmd, expected string, seconds int) {
	ginkgo.By(description)
	defer kubectl.DumpLogs(ns.genTestLogDir(description), ns.Namespace).ExecVPanic()
	execErr := ns.WaitForOutputPattern(kcmd, expected, seconds)
	Expect(execErr).ToNot(HaveOccurred())
}

func (ns *NsWrapper) WaitForOutputContainsAndLog(description string, kcmd kubectl.KCmd, expected string, seconds int) {
	ginkgo.By(description)
	defer kubectl.DumpLogs(ns.genTestLogDir(description), ns.Namespace).ExecVPanic()
	execErr := ns.WaitForOutputContains(kcmd, expected, seconds)
	Expect(execErr).ToNot(HaveOccurred())
}

func (ns *NsWrapper) WaitForDatacenterCondition(dcName string, conditionType string, value string) {
	ns.WaitForDatacenterConditionWithTimeout(dcName, conditionType, value, 600)
}

func (ns *NsWrapper) WaitForDatacenterConditionWithTimeout(dcName, conditionType, value string, seconds int) {
	step := fmt.Sprintf("checking that dc condition %s has value %s", conditionType, value)
	json := fmt.Sprintf("jsonpath={.status.conditions[?(.type=='%s')].status}", conditionType)
	k := kubectl.Get("cassandradatacenter", dcName).
		FormatOutput(json)
	ns.WaitForOutputAndLog(step, k, value, seconds)
}

func (ns *NsWrapper) WaitForDatacenterConditionWithReason(dcName string, conditionType string, value string, reason string) {
	step := fmt.Sprintf("checking that dc condition %s has value %s", conditionType, value)
	json := fmt.Sprintf("jsonpath={.status.conditions[?(.type=='%s')].status}", conditionType)
	k := kubectl.Get("cassandradatacenter", dcName).
		FormatOutput(json)
	ns.WaitForOutputAndLog(step, k, value, 600)
}

func (ns *NsWrapper) WaitForDatacenterToHaveNoPods(dcName string) {
	step := "checking that no dc pods remain"
	json := "jsonpath={.items}"
	k := kubectl.Get("pods").
		WithLabel(fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)).
		FormatOutput(json)
	ns.WaitForOutputAndLog(step, k, "[]", 300)
}

func (ns *NsWrapper) WaitForDatacenterOperatorProgress(dcName string, progressValue string, timeout int) {
	step := fmt.Sprintf("checking the cassandra operator progress status is set to %s", progressValue)
	json := "jsonpath={.status.cassandraOperatorProgress}"
	k := kubectl.Get("CassandraDatacenter", dcName).
		FormatOutput(json)
	ns.WaitForOutputAndLog(step, k, progressValue, timeout)
}

func (ns *NsWrapper) WaitForSuperUserUpserted(dcName string, timeout int) {
	json := "jsonpath={.status.superUserUpserted}"
	k := kubectl.Get("CassandraDatacenter", dcName).
		FormatOutput(json)
	execErr := ns.WaitForOutputPattern(k, `\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z`, timeout)
	Expect(execErr).ToNot(HaveOccurred())
}

func (ns *NsWrapper) GetNodeStatusesHostIds(dcName string) []string {
	json := "jsonpath={.status.nodeStatuses['*'].hostID}"
	k := kubectl.Get("CassandraDatacenter", dcName).
		FormatOutput(json)

	output := ns.OutputPanic(k)
	hostIds := strings.Split(output, " ")

	return hostIds
}

func (ns *NsWrapper) WaitForDatacenterReadyPodCount(dcName string, count int) {
	ns.WaitForDatacenterReadyPodCountWithTimeout(dcName, count, 600)
}

func (ns *NsWrapper) WaitForDatacenterReadyPodCountWithTimeout(dcName string, count int, podCountTimeout int) {
	timeout := count * podCountTimeout
	step := "waiting for the node to become ready"
	json := "jsonpath={.items[*].status.containerStatuses[0].ready}"
	k := kubectl.Get("pods").
		WithLabel(fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)).
		WithFlag("field-selector", "status.phase=Running").
		FormatOutput(json)
	ns.WaitForOutputAndLog(step, k, duplicate("true", count), timeout)
}

func (ns *NsWrapper) WaitForDatacenterReady(dcName string) {
	ns.WaitForDatacenterReadyWithTimeouts(dcName, 600, 60)
}

func (ns *NsWrapper) WaitForDatacenterReadyWithTimeouts(dcName string, podCountTimeout int, dcReadyTimeout int) {
	json := "jsonpath={.spec.size}"
	k := kubectl.Get("CassandraDatacenter", dcName).FormatOutput(json)
	sizeString := ns.OutputPanic(k)
	size, err := strconv.Atoi(sizeString)
	Expect(err).ToNot(HaveOccurred())

	ns.WaitForDatacenterReadyPodCountWithTimeout(dcName, size, podCountTimeout)
	ns.WaitForDatacenterOperatorProgress(dcName, "Ready", dcReadyTimeout)
}

func (ns *NsWrapper) WaitForPodNotStarted(podName string) {
	step := "verify that the pod is no longer marked as started"
	k := kubectl.Get("pod").
		WithFlag("field-selector", "metadata.name="+podName).
		WithFlag("selector", "cassandra.datastax.com/node-state=Started")
	ns.WaitForOutputAndLog(step, k, "", 60)
}

func (ns *NsWrapper) WaitForPodStarted(podName string) {
	step := "verify that the pod is marked as started"
	json := "jsonpath={.items[*].metadata.name}"
	k := kubectl.Get("pod").
		WithFlag("field-selector", "metadata.name="+podName).
		WithFlag("selector", "cassandra.datastax.com/node-state=Started").
		FormatOutput(json)
	ns.WaitForOutputAndLog(step, k, podName, 60)
}

func (ns *NsWrapper) WaitForCassandraImages(dcName string, expectedImages []string, timeout int) {
	step := "verify cassandra image updates"
	images := strings.Join(expectedImages, " ")
	json := "jsonpath={.items[*].spec.containers[?(@.name == 'cassandra')].image}"
	k := kubectl.Get("pods").
		WithFlag("selector", fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)).
		FormatOutput(json)
	ns.WaitForOutputAndLog(step, k, images, timeout)
}

func (ns *NsWrapper) DisableGossipWaitNotReady(podName string) {
	ns.DisableGossip(podName)
	ns.WaitForPodNotStarted(podName)
}

func (ns *NsWrapper) EnableGossipWaitReady(podName string) {
	ns.EnableGossip(podName)
	ns.WaitForPodStarted(podName)
}

func (ns *NsWrapper) DisableGossip(podName string) {
	execArgs := []string{"-c", "cassandra",
		"--", "bash", "-c",
		"nodetool disablegossip",
	}
	k := kubectl.ExecOnPod(podName, execArgs...)
	ns.ExecVPanic(k)
}

func (ns *NsWrapper) EnableGossip(podName string) {
	execArgs := []string{"-c", "cassandra",
		"--", "bash", "-c",
		"nodetool enablegossip",
	}
	k := kubectl.ExecOnPod(podName, execArgs...)
	ns.ExecVPanic(k)
}

func (ns *NsWrapper) GetDatacenterPodNames(dcName string) []string {
	json := "jsonpath={.items[*].metadata.name}"
	k := kubectl.Get("pods").
		WithFlag("selector", fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)).
		FormatOutput(json)

	output := ns.OutputPanic(k)
	podNames := strings.Split(output, " ")
	sort.Strings(podNames)

	return podNames
}

func (ns *NsWrapper) GetDatacenterReadyPodNames(dcName string) []string {
	json := "jsonpath={.items[?(@.status.containerStatuses[0].ready==true)].metadata.name}"
	k := kubectl.Get("pods").
		WithFlag("selector", fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)).
		FormatOutput(json)

	output := ns.OutputPanic(k)
	podNames := strings.Split(output, " ")
	sort.Strings(podNames)

	return podNames
}

func (ns *NsWrapper) GetCassandraContainerImages(dcName string) []string {
	json := "jsonpath={.items[*].spec.containers[?(@.name == 'cassandra')].image}"
	k := kubectl.Get("pods").
		WithFlag("selector", fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)).
		FormatOutput(json)

	output := ns.OutputPanic(k)
	images := strings.Split(output, " ")
	sort.Strings(images)

	return images
}

func (ns *NsWrapper) WaitForOperatorReady() {
	step := "waiting for the operator to become ready"
	json := "jsonpath={.items[0].status.containerStatuses[0].ready}"
	k := kubectl.Get("pods").
		WithLabel("name=cass-operator").
		WithFlag("field-selector", "status.phase=Running").
		FormatOutput(json)
	ns.WaitForOutputAndLog(step, k, "true", 300)
}

// kubectl create secret docker-registry github-docker-registry --docker-username=USER --docker-password=PASS --docker-server docker.pkg.github.com
func CreateDockerRegistrySecret(name string, namespace string) {
	args := []string{"secret", "docker-registry", name}
	flags := map[string]string{
		"docker-username": os.Getenv(kubectl.EnvDockerUsername),
		"docker-password": os.Getenv(kubectl.EnvDockerPassword),
		"docker-server":   os.Getenv(kubectl.EnvDockerServer),
	}
	k := kubectl.KCmd{Command: "create", Args: args, Flags: flags}
	k.InNamespace(namespace).ExecVCapture()
}

// Note that the actual value will be cast to a string before the comparison with the expectedValue
func (ns NsWrapper) ExpectKeyValue(m map[string]interface{}, key string, expectedValue string) {
	actualValue, ok := m[key].(string)
	if !ok {
		// Note: floats will end up as strings with six decimal points
		// example: "12.000000"
		tryFloat64, ok := m[key].(float64)
		if !ok {
			msg := fmt.Sprintf("Actual value for key %s is not expected type", key)
			err := fmt.Errorf(msg)
			Expect(err).ToNot(HaveOccurred())
		}
		actualValue = fmt.Sprintf("%f", tryFloat64)
	}
	Expect(actualValue).To(Equal(expectedValue), "Expected %s %s to be %s", key, m[key], expectedValue)
}

// Compare all key/values from an expected map to an actual map
func (ns NsWrapper) ExpectKeyValues(actual map[string]interface{}, expected map[string]string) {
	for key := range expected {
		ns.ExpectKeyValue(actual, key, expected[key])
	}
}

func (ns NsWrapper) ExpectDoneReconciling(dcName string) {
	ginkgo.By(fmt.Sprintf("ensure %s is done reconciling", dcName))
	time.Sleep(1 * time.Minute)

	json := `jsonpath={.metadata.resourceVersion}`
	k := kubectl.Get("CassandraDatacenter", dcName).
		FormatOutput(json)
	resourceVersion := ns.OutputPanic(k)

	time.Sleep(1 * time.Minute)

	json = `jsonpath={.metadata.resourceVersion}`
	k = kubectl.Get("CassandraDatacenter", dcName).
		FormatOutput(json)
	newResourceVersion := ns.OutputPanic(k)

	Expect(newResourceVersion).To(Equal(resourceVersion),
		"CassandraDatacenter %s is still being reconciled as the resource version is changing", dcName)
}

type NodetoolNodeInfo struct {
	Status  string
	State   string
	Address string
	HostId  string
	Rack    string
}

func (ns NsWrapper) RetrieveStatusFromNodetool(podName string) []NodetoolNodeInfo {
	k := kubectl.KCmd{Command: "exec", Args: []string{podName, "-i", "-c", "cassandra", "--namespace", ns.Namespace, "--", "nodetool", "status"}}
	output, err := k.Output()
	Expect(err).ToNot(HaveOccurred())

	getFullName := func(s string) string {
		status, ok := map[string]string{
			"U": "up",
			"D": "down",
			"N": "normal",
			"L": "leaving",
			"J": "joining",
			"M": "moving",
			"S": "stopped",
		}[string(s)]

		if !ok {
			status = s
		}
		return status
	}

	nodeTexts := regexp.MustCompile(`(?m)^.*(([0-9a-fA-F]+-){4}([0-9a-fA-F]+)).*$`).FindAllString(output, -1)
	nodeInfo := []NodetoolNodeInfo{}
	for _, nodeText := range nodeTexts {
		comps := regexp.MustCompile(`[[:space:]]+`).Split(strings.TrimSpace(nodeText), -1)
		nodeInfo = append(nodeInfo,
			NodetoolNodeInfo{
				Status:  getFullName(string(comps[0][0])),
				State:   getFullName(string(comps[0][1])),
				Address: comps[1],
				HostId:  comps[len(comps)-2],
				Rack:    comps[len(comps)-1],
			})
	}
	return nodeInfo
}

func (ns NsWrapper) RetrieveSuperuserCreds(clusterName string) (string, string) {
	secretName := fmt.Sprintf("%s-superuser", clusterName)
	secretResource := fmt.Sprintf("secret/%s", secretName)

	ginkgo.By("get superuser username")
	json := "jsonpath={.data.username}"
	k := kubectl.Get(secretResource).FormatOutput(json)
	usernameBase64 := ns.OutputPanic(k)
	Expect(usernameBase64).ToNot(Equal(""), "Expected secret to specify a username")
	usernameDecoded, err := base64.StdEncoding.DecodeString(usernameBase64)
	Expect(err).ToNot(HaveOccurred())

	ginkgo.By("get superuser password")
	json = "jsonpath={.data.password}"
	k = kubectl.Get(secretResource).FormatOutput(json)
	passwordBase64 := ns.OutputPanic(k)
	Expect(passwordBase64).ToNot(Equal(""), "Expected secret to specify a password")
	passwordDecoded, err := base64.StdEncoding.DecodeString(passwordBase64)
	Expect(err).ToNot(HaveOccurred())

	return string(usernameDecoded), string(passwordDecoded)
}

func (ns NsWrapper) CqlExecute(podName string, stepDesc string, cql string, user string, pw string) {
	k := kubectl.ExecOnPod(
		podName, "--", "cqlsh",
		"--user", user,
		"--password", pw,
		"-e", cql).
		WithFlag("container", "cassandra")
	ginkgo.By(stepDesc)
	ns.ExecVPanic(k)
}

func (ns *NsWrapper) CheckForCompletedCassandraTasks(dcName, command string, count int) {
	step := fmt.Sprintf("checking that cassandratask command %s has succeeded", command)
	json := "jsonpath={.items[*].spec.jobs[0].command}"
	k := kubectl.Get("cassandratask").
		WithLabel(fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)).
		WithLabel("control.k8ssandra.io/status=completed").
		FormatOutput(json)
	ns.WaitForOutputAndLog(step, k, duplicate(command, count), 120)
}
