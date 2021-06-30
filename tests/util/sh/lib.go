// Copyright DataStax, Inc.
// Please see the included license file for details.

package shutil

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"

	mageutil "github.com/k8ssandra/cass-operator/tests/util"
)

// Run command.
//
// If mage is run with -v flag, stdout will be used
// to print output, if not, output will be hidden.
// Stderr will work as normal
func Run(cmd string, args ...string) error {
	return RunWithEnv(nil, cmd, args...)
}

func RunWithEnv(env map[string]string, cmd string, args ...string) error {
	var output io.Writer
	_, err := Exec(env, output, os.Stderr, cmd, args...)
	return err
}

// Run command.
//
// If mage is run with -v flag, stdout will be used
// to print output, if not, output will be hidden.
// Stderr will work as normal
// Will automatically panic on error
func RunPanic(cmd string, args ...string) {
	err := Run(cmd, args...)
	mageutil.PanicOnError(err)
}

// Run command and print any output to stdout/stderr
func RunV(cmd string, args ...string) error {
	return RunVWithEnv(nil, cmd, args...)
}

func RunVWithEnv(env map[string]string, cmd string, args ...string) error {
	_, err := Exec(env, os.Stdout, os.Stderr, cmd, args...)
	return err
}

// Run command and print any output to stdout/stderr
// Will automatically panic on error
func RunVPanic(cmd string, args ...string) {
	RunVPanicWithEnv(nil, cmd, args...)
}

func RunVPanicWithEnv(env map[string]string, cmd string, args ...string) {
	err := RunVWithEnv(env, cmd, args...)
	mageutil.PanicOnError(err)
}

// Run command and print any output to stdout/stderr
// Also return stdout/stderr as strings
func RunVCapture(cmd string, args ...string) (string, string, error) {
	captureOut := new(bytes.Buffer)
	captureErr := new(bytes.Buffer)

	// Duplicate the output/error to our buffer and the test stdout/stderr
	multiOut := io.MultiWriter(captureOut, os.Stdout)
	multiErr := io.MultiWriter(captureErr, os.Stderr)

	_, err := Exec(nil, multiOut, multiErr, cmd, args...)
	return captureOut.String(), captureErr.String(), err
}

// Returns output from stdout.
// stderr gets used as normal here
func Output(cmd string, args ...string) (string, error) {
	return OutputWithEnv(nil, cmd, args...)
}

func OutputWithEnv(env map[string]string, cmd string, args ...string) (string, error) {
	buf := &bytes.Buffer{}
	_, err := Exec(env, buf, os.Stderr, cmd, args...)
	return strings.TrimSuffix(buf.String(), "\n"), err
}

// Returns output from stdout, and panics on error
// stderr gets used as normal here
func OutputPanic(cmd string, args ...string) string {
	out, err := Output(cmd, args...)
	mageutil.PanicOnError(err)
	return out
}

func cmdWithStdIn(env map[string]string, cmd string, in string, args ...string) *exec.Cmd {
	envArray := []string{}
	for k, v := range env {
		envArray = append(envArray, fmt.Sprintf("%s=%s", k, v))
	}
	c := exec.Command(cmd, args...)
	c.Env = envArray
	buffer := bytes.Buffer{}
	buffer.Write([]byte(in))
	c.Stdin = &buffer
	return c
}

func RunWithInput(cmd string, in string, args ...string) error {
	return RunWithEnvWithInput(nil, cmd, in, args...)
}

func RunWithEnvWithInput(env map[string]string, cmd string, in string, args ...string) error {
	c := cmdWithStdIn(nil, cmd, in, args...)
	var output io.Writer
	c.Stderr = os.Stderr
	c.Stdout = output
	return c.Run()
}

func RunVWithInput(cmd string, in string, args ...string) error {
	return RunVWithEnvWithInput(nil, cmd, in, args...)
}

func RunVWithEnvWithInput(env map[string]string, cmd string, in string, args ...string) error {
	c := cmdWithStdIn(env, cmd, in, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func OutputWithInput(cmd string, in string, args ...string) (string, error) {
	return OutputWithEnvWithInput(nil, cmd, in, args...)
}

func OutputWithEnvWithInput(env map[string]string, cmd string, in string, args ...string) (string, error) {
	envArray := []string{}
	for k, v := range env {
		envArray = append(envArray, fmt.Sprintf("%s=%s", k, v))
	}
	c := exec.Command(cmd, args...)
	c.Env = envArray
	buffer := bytes.Buffer{}
	buffer.Write([]byte(in))
	c.Stdin = &buffer
	out, err := c.Output()
	return string(out), err
}

// Copied from mage
func Exec(env map[string]string, stdout, stderr io.Writer, cmd string, args ...string) (ran bool, err error) {
	expand := func(s string) string {
		s2, ok := env[s]
		if ok {
			return s2
		}
		return os.Getenv(s)
	}
	cmd = os.Expand(cmd, expand)
	for i := range args {
		args[i] = os.Expand(args[i], expand)
	}
	ran, code, err := run(env, stdout, stderr, cmd, args...)
	if err == nil {
		return true, nil
	}
	if ran {
		return ran, fmt.Errorf(`running "%s %s" failed with exit code %d`, cmd, strings.Join(args, " "), code)
	}
	return ran, fmt.Errorf(`failed to run "%s %s: %v"`, cmd, strings.Join(args, " "), err)
}

func run(env map[string]string, stdout, stderr io.Writer, cmd string, args ...string) (ran bool, code int, err error) {
	c := exec.Command(cmd, args...)
	c.Env = os.Environ()
	for k, v := range env {
		c.Env = append(c.Env, k+"="+v)
	}
	c.Stderr = stderr
	c.Stdout = stdout
	c.Stdin = os.Stdin
	log.Println("exec:", cmd, strings.Join(args, " "))
	err = c.Run()
	return CmdRan(err), ExitStatus(err), err
}

func CmdRan(err error) bool {
	if err == nil {
		return true
	}
	ee, ok := err.(*exec.ExitError)
	if ok {
		return ee.Exited()
	}
	return false
}

type exitStatus interface {
	ExitStatus() int
}

// ExitStatus returns the exit status of the error if it is an exec.ExitError
// or if it implements ExitStatus() int.
// 0 if it is nil or 1 if it is a different error.
func ExitStatus(err error) int {
	if err == nil {
		return 0
	}
	if e, ok := err.(exitStatus); ok {
		return e.ExitStatus()
	}
	if e, ok := err.(*exec.ExitError); ok {
		if ex, ok := e.Sys().(exitStatus); ok {
			return ex.ExitStatus()
		}
	}
	return 1
}
