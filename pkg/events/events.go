package events

import (
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	record "k8s.io/client-go/tools/events"
)

const (
	// Events
	UpdatingRack                      string = "UpdatingRack"
	StoppingDatacenter                string = "StoppingDatacenter"
	DeletingStuckPod                  string = "DeletingStuckPod"
	RestartingCassandra               string = "RestartingCassandra"
	CreatedResource                   string = "CreatedResource"
	StartedCassandra                  string = "StartedCassandra"
	LabeledPodAsSeed                  string = "LabeledPodAsSeed"
	LabeledPodAsDecommissioning       string = "LabeledPodAsDecommissioning"
	DeletedPvc                        string = "DeletedPvc"
	ResizingPVC                       string = "ResizingPVC"
	ResizingPVCFailed                 string = "ResizingPVCFailed"
	UnlabeledPodAsSeed                string = "UnlabeledPodAsSeed"
	LabeledRackResource               string = "LabeledRackResource"
	ScalingUpRack                     string = "ScalingUpRack"
	ScalingDownRack                   string = "ScalingDownRack"
	CreatedSuperuser                  string = "CreatedSuperuser" // deprecated
	CreatedUsers                      string = "CreatedUsers"
	FinishedReplaceNode               string = "FinishedReplaceNode"
	ReplacingNode                     string = "ReplacingNode"
	StartingCassandraAndReplacingNode string = "StartingCassandraAndReplacingNode"
	StartingCassandra                 string = "StartingCassandra"
	DecommissionDatacenter            string = "DecommissionDatacenter"
	DecommissioningNode               string = "DecommissioningNode"
	UnhealthyDatacenter               string = "UnhealthyDatacenter"
	RecreatingStatefulSet             string = "RecreatingStatefulSet"
	InvalidDatacenterSpec             string = "InvalidDatacenterSpec"
)

type LoggingEventRecorder struct {
	record.EventRecorderLogger
	ReqLogger logr.Logger
}

// Eventf is just a wrapper to do WithLogger always.
// Few notes for caller:
// action is a constant from this file and is machine readable.
// reason and note are human readable. Reason is short and note can include longer description with arguments
func (r *LoggingEventRecorder) Eventf(object runtime.Object, related runtime.Object, eventtype, reason, action, note string, args ...any) {
	r.EventRecorderLogger.WithLogger(r.ReqLogger).Eventf(object, related, eventtype, reason, action, note, args...)
}

// Event is a simplified version of Eventf with no support for related or note. Action is machine readable from this file
// and reason has ability to use args. This is for backwards compatibility
func (r *LoggingEventRecorder) Event(object runtime.Object, eventtype, action, reason string, args ...any) {
	r.Eventf(object, nil, eventtype, fmt.Sprintf(reason, args...), action, "")
}
