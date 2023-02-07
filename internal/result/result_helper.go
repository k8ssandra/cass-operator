// Copyright DataStax, Inc.
// Please see the included license file for details.

package result

import (
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

var DurationFunc func(int) time.Duration = func(secs int) time.Duration { return time.Duration(secs) * time.Second }

type ReconcileResult interface {
	Completed() bool
	Output() (ctrl.Result, error)
}

type continueReconcile struct{}

func (c continueReconcile) Completed() bool {
	return false
}
func (c continueReconcile) Output() (ctrl.Result, error) {
	panic("there was no Result to return")
}

type done struct{}

func (d done) Completed() bool {
	return true
}
func (d done) Output() (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

type callBackSoon struct {
	secs int
}

func (c callBackSoon) Completed() bool {
	return true
}
func (c callBackSoon) Output() (ctrl.Result, error) {
	t := DurationFunc(c.secs)
	return ctrl.Result{Requeue: true, RequeueAfter: t}, nil
}

type errorOut struct {
	err error
}

func (e errorOut) Completed() bool {
	return true
}
func (e errorOut) Output() (ctrl.Result, error) {
	return ctrl.Result{}, e.err
}

func Continue() ReconcileResult {
	return continueReconcile{}
}

func Done() ReconcileResult {
	return done{}
}

func RequeueSoon(secs int) ReconcileResult {
	return callBackSoon{secs: secs}
}

func Error(e error) ReconcileResult {
	return errorOut{err: e}
}
