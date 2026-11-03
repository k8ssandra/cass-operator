// Copyright DataStax, Inc.
// Please see the included license file for details.

package ginkgo_util

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	api "github.com/k8ssandra/cass-operator/apis/cassandra/v1beta1"
	"github.com/k8ssandra/cass-operator/tests/util/kubectl"
)

const (
	nodeStateLabel      = "cassandra.datastax.com/node-state"
	watchedByAnnotation = "cassandra.datastax.com/watched-by"
)

// invariantOptions changes what AssertDatacenterInvariants checks. The defaults are strict, an option only relaxes a
// check that is known not to hold in that step of a test.
type invariantOptions struct {
	allowedWarningReasons map[string]bool
	skipRevisionCheck     bool
	skipRingCheck         bool
	skipQuietCheck        bool
}

type InvariantOption func(*invariantOptions)

// AllowWarningEvents allows Warning events with the given reasons since the last checkpoint, for steps that break the
// datacenter on purpose.
func AllowWarningEvents(reasons ...string) InvariantOption {
	return func(o *invariantOptions) {
		for _, reason := range reasons {
			o.allowedWarningReasons[reason] = true
		}
	}
}

// SkipStatefulSetRevisionCheck does not require every StatefulSet to be on its update revision, for canary upgrades.
func SkipStatefulSetRevisionCheck() InvariantOption {
	return func(o *invariantOptions) {
		o.skipRevisionCheck = true
	}
}

// SkipRingCheck does not compare the ring (nodetool status) with the datacenter status.
func SkipRingCheck() InvariantOption {
	return func(o *invariantOptions) {
		o.skipRingCheck = true
	}
}

// SkipQuietCheck does not wait for the operator to stop reconciling the datacenter.
func SkipQuietCheck() InvariantOption {
	return func(o *invariantOptions) {
		o.skipQuietCheck = true
	}
}

var (
	eventCheckpointsMu sync.Mutex
	// eventCheckpoints has the time of the last invariant check per namespace/datacenter, Warning events before it
	// have been checked already.
	eventCheckpoints = map[string]time.Time{}
)

func (ns NsWrapper) eventCheckpoint(dcName string) (time.Time, func(time.Time)) {
	key := ns.Namespace + "/" + dcName
	eventCheckpointsMu.Lock()
	defer eventCheckpointsMu.Unlock()
	return eventCheckpoints[key], func(t time.Time) {
		eventCheckpointsMu.Lock()
		defer eventCheckpointsMu.Unlock()
		eventCheckpoints[key] = t
	}
}

// ResetEventCheckpoint marks the Warning events of the datacenter up to now as checked. Use it after a step that is
// expected to cause Warning events and that has its own assertions for them.
func (ns NsWrapper) ResetEventCheckpoint(dcName string) {
	_, set := ns.eventCheckpoint(dcName)
	set(time.Now())
}

func (ns NsWrapper) getJson(into any, kcmd kubectl.KCmd) {
	out, err := ns.Output(kcmd.FormatOutput("json"))
	Expect(err).ToNot(HaveOccurred())
	Expect(json.Unmarshal([]byte(out), into)).To(Succeed())
}

// GetDatacenter returns the CassandraDatacenter as stored in the cluster
func (ns NsWrapper) GetDatacenter(dcName string) *api.CassandraDatacenter {
	dc := &api.CassandraDatacenter{}
	ns.getJson(dc, kubectl.Get("CassandraDatacenter", dcName))
	return dc
}

func dcSelector(dcName string) string {
	return fmt.Sprintf("%s=%s", api.DatacenterLabel, api.CleanLabelValue(dcName))
}

// GetDatacenterPods returns the pods of the datacenter
func (ns NsWrapper) GetDatacenterPods(dcName string) []corev1.Pod {
	pods := &corev1.PodList{}
	ns.getJson(pods, kubectl.Get("pods").WithLabel(dcSelector(dcName)))
	return pods.Items
}

// GetDatacenterStatefulSets returns the StatefulSets of the datacenter
func (ns NsWrapper) GetDatacenterStatefulSets(dcName string) []appsv1.StatefulSet {
	sts := &appsv1.StatefulSetList{}
	ns.getJson(sts, kubectl.Get("statefulsets").WithLabel(dcSelector(dcName)))
	return sts.Items
}

func isPodReady(pod *corev1.Pod) bool {
	if pod.DeletionTimestamp != nil {
		return false
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == "cassandra" {
			return cs.Ready
		}
	}
	return false
}

// DatacenterState is a snapshot of the identity of a datacenter, to compare before and after an operation
type DatacenterState struct {
	// HostIDs per pod name, from the CassandraDatacenter status
	HostIDs map[string]string
	// PodUIDs per pod name
	PodUIDs map[string]string
	// UpdateRevisions per StatefulSet name
	UpdateRevisions map[string]string
}

// GetDatacenterState records the HostIDs, pod UIDs and StatefulSet revisions of the datacenter
func (ns NsWrapper) GetDatacenterState(dcName string) DatacenterState {
	state := DatacenterState{
		HostIDs:         map[string]string{},
		PodUIDs:         map[string]string{},
		UpdateRevisions: map[string]string{},
	}
	dc := ns.GetDatacenter(dcName)
	for pod, status := range dc.Status.NodeStatuses {
		state.HostIDs[pod] = status.HostID
	}
	for _, pod := range ns.GetDatacenterPods(dcName) {
		state.PodUIDs[pod.Name] = string(pod.UID)
	}
	for _, sts := range ns.GetDatacenterStatefulSets(dcName) {
		state.UpdateRevisions[sts.Name] = sts.Status.UpdateRevision
	}
	return state
}

// ExpectSameHostIDs fails if any node of the datacenter has a different HostID than in the earlier state, which
// means it bootstrapped again.
func (ns NsWrapper) ExpectSameHostIDs(dcName string, before DatacenterState) {
	ginkgo.By(fmt.Sprintf("checking that the HostIDs of %s did not change", dcName))
	after := ns.GetDatacenterState(dcName)
	Expect(after.HostIDs).To(Equal(before.HostIDs), "HostIDs changed, a node bootstrapped again")
}

// AssertDatacenterInvariants checks what must hold for a datacenter that is Ready and not changing: conditions,
// status fields, pods, StatefulSets, the ring, Warning events since the last check and a quiet reconcile loop.
func (ns NsWrapper) AssertDatacenterInvariants(dcName string, opts ...InvariantOption) {
	o := &invariantOptions{allowedWarningReasons: map[string]bool{}}
	for _, opt := range opts {
		opt(o)
	}

	ginkgo.By(fmt.Sprintf("checking the invariants of datacenter %s", dcName))
	checkpoint, setCheckpoint := ns.eventCheckpoint(dcName)
	checkTime := time.Now()

	if !o.skipQuietCheck {
		ns.ExpectDoneReconciling(dcName)
	}

	dc := ns.GetDatacenter(dcName)

	// Conditions
	for _, condition := range []api.DatacenterConditionType{api.DatacenterReady, api.DatacenterInitialized, api.DatacenterValid, api.DatacenterHealthy} {
		Expect(dc.GetConditionStatus(condition)).To(Equal(corev1.ConditionTrue), "condition %s of %s", condition, dcName)
	}
	for _, condition := range []api.DatacenterConditionType{api.DatacenterScalingUp, api.DatacenterScalingDown, api.DatacenterUpdating,
		api.DatacenterReplacingNodes, api.DatacenterRollingRestart, api.DatacenterStopped, api.DatacenterResuming, api.DatacenterResizingVolumes} {
		if status, found := dc.GetCondition(condition); found {
			Expect(status.Status).To(Equal(corev1.ConditionFalse), "condition %s of %s", condition, dcName)
		}
	}

	// Status
	Expect(dc.Status.CassandraOperatorProgress).To(Equal(api.ProgressReady), "cassandraOperatorProgress of %s", dcName)
	Expect(dc.Status.ObservedGeneration).To(Equal(dc.Generation), "status.observedGeneration of %s", dcName)
	Expect(dc.Status.FailedStarts).To(BeEmpty(), "status.failedStarts of %s", dcName)
	Expect(dc.Status.NodeReplacements).To(BeEmpty(), "status.nodeReplacements of %s", dcName)
	Expect(dc.Status.TrackedTasks).To(BeEmpty(), "status.trackedTasks of %s", dcName)

	// Pods
	pods := ns.GetDatacenterPods(dcName)
	Expect(pods).To(HaveLen(int(dc.Spec.Size)), "pods of %s", dcName)
	hostIDs := []string{}
	for i := range pods {
		pod := &pods[i]
		Expect(pod.Labels).To(HaveKeyWithValue(nodeStateLabel, "Started"), "node-state label of pod %s", pod.Name)
		Expect(isPodReady(pod)).To(BeTrue(), "pod %s is not Ready", pod.Name)
		status, found := dc.Status.NodeStatuses[pod.Name]
		Expect(found).To(BeTrue(), "pod %s has no status.nodeStatuses entry", pod.Name)
		Expect(status.HostID).ToNot(BeEmpty(), "pod %s has no HostID in status.nodeStatuses", pod.Name)
		hostIDs = append(hostIDs, status.HostID)
	}

	// StatefulSets
	for _, sts := range ns.GetDatacenterStatefulSets(dcName) {
		Expect(sts.Status.ObservedGeneration).To(Equal(sts.Generation), "StatefulSet %s status.observedGeneration", sts.Name)
		Expect(sts.Status.ReadyReplicas).To(Equal(*sts.Spec.Replicas), "StatefulSet %s readyReplicas", sts.Name)
		if !o.skipRevisionCheck {
			Expect(sts.Status.CurrentRevision).To(Equal(sts.Status.UpdateRevision), "StatefulSet %s is not on its update revision", sts.Name)
			Expect(sts.Status.UpdatedReplicas).To(Equal(*sts.Spec.Replicas), "StatefulSet %s updatedReplicas", sts.Name)
		}
	}

	// Ring
	if !o.skipRingCheck && len(pods) > 0 {
		nodes := ns.RetrieveDatacenterStatusFromNodetool(pods[0].Name)
		ring, found := nodes[dc.DatacenterName()]
		Expect(found).To(BeTrue(), "nodetool status on %s does not show datacenter %s: %v", pods[0].Name, dc.DatacenterName(), nodes)
		ringHostIDs := []string{}
		for _, node := range ring {
			Expect(node.Status+"/"+node.State).To(Equal("up/normal"), "node %s (%s) in nodetool status", node.Address, node.HostId)
			ringHostIDs = append(ringHostIDs, node.HostId)
		}
		Expect(ringHostIDs).To(ConsistOf(hostIDs), "the ring of %s does not match status.nodeStatuses", dcName)
	}

	// Warning events
	warnings := ns.WarningEventsSince(dcName, checkpoint)
	unexpected := []string{}
	for _, event := range warnings {
		if !o.allowedWarningReasons[event.Reason] {
			unexpected = append(unexpected, fmt.Sprintf("%s: %s", event.Reason, event.Message))
		}
	}
	Expect(unexpected).To(BeEmpty(), "Warning events for %s since %s", dcName, checkpoint.Format(time.RFC3339))

	setCheckpoint(checkTime)
}

func eventTime(event *corev1.Event) time.Time {
	t := event.LastTimestamp.Time
	if event.Series != nil && event.Series.LastObservedTime.After(t) {
		t = event.Series.LastObservedTime.Time
	}
	if event.EventTime.After(t) {
		t = event.EventTime.Time
	}
	if t.IsZero() {
		t = event.CreationTimestamp.Time
	}
	return t
}

// WarningEventsSince returns the Warning events of the CassandraDatacenter that happened after the given time
func (ns NsWrapper) WarningEventsSince(dcName string, since time.Time) []corev1.Event {
	events := &corev1.EventList{}
	ns.getJson(events, kubectl.Get("events").
		WithFlag("field-selector", fmt.Sprintf("involvedObject.kind=CassandraDatacenter,involvedObject.name=%s,type=Warning", dcName)))

	result := []corev1.Event{}
	for i := range events.Items {
		// Event timestamps have a resolution of a second
		if !eventTime(&events.Items[i]).Before(since.Truncate(time.Second)) {
			result = append(result, events.Items[i])
		}
	}
	return result
}

// WaitForWarningEvent waits for a Warning event of the CassandraDatacenter with the given reason and a message
// matching the pattern, after the given time.
func (ns NsWrapper) WaitForWarningEvent(dcName string, since time.Time, reason, messagePattern string, timeout time.Duration) {
	ginkgo.By(fmt.Sprintf("waiting for a %s Warning event for %s", reason, dcName))
	re := regexp.MustCompile(messagePattern)
	Eventually(func() bool {
		for _, event := range ns.WarningEventsSince(dcName, since) {
			if event.Reason == reason && re.MatchString(event.Message) {
				return true
			}
		}
		return false
	}).WithTimeout(timeout).WithPolling(5*time.Second).Should(BeTrue(), "no %s Warning event matching %q", reason, messagePattern)
}

// RetrieveDatacenterStatusFromNodetool runs nodetool status on the pod and returns the nodes per datacenter
func (ns NsWrapper) RetrieveDatacenterStatusFromNodetool(podName string) map[string][]NodetoolNodeInfo {
	k := kubectl.ExecOnPod(podName, "-c", "cassandra", "--", "nodetool", "status")
	output, err := ns.Output(k)
	Expect(err).ToNot(HaveOccurred())
	return parseNodetoolStatus(output)
}

var (
	nodetoolDatacenterLine = regexp.MustCompile(`^Datacenter:\s*(\S+)`)
	nodetoolNodeLine       = regexp.MustCompile(`^([UD][NLJMS])\s+(\S+)\s.*\s(([0-9a-fA-F]+-){4}[0-9a-fA-F]+)\s+(\S+)\s*$`)
)

func parseNodetoolStatus(output string) map[string][]NodetoolNodeInfo {
	states := map[byte]string{'U': "up", 'D': "down", 'N': "normal", 'L': "leaving", 'J': "joining", 'M': "moving", 'S': "stopped"}
	result := map[string][]NodetoolNodeInfo{}
	dc := ""
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if m := nodetoolDatacenterLine.FindStringSubmatch(line); m != nil {
			dc = m[1]
			continue
		}
		if m := nodetoolNodeLine.FindStringSubmatch(line); m != nil {
			result[dc] = append(result[dc], NodetoolNodeInfo{
				Status:  states[m[1][0]],
				State:   states[m[1][1]],
				Address: m[2],
				HostId:  m[3],
				Rack:    m[5],
			})
		}
	}
	return result
}

// AssertDatacenterDeleted waits for the CassandraDatacenter to be gone and checks that it left nothing behind:
// no pods, PVCs, StatefulSets, Services or PodDisruptionBudgets, and no Secret still watched by it.
func (ns NsWrapper) AssertDatacenterDeleted(dcName string) {
	ginkgo.By(fmt.Sprintf("checking that datacenter %s was deleted with everything it owned", dcName))
	selector := dcSelector(dcName)

	Eventually(func() string {
		out, _ := ns.Output(kubectl.Get("CassandraDatacenter").WithFlag("field-selector", "metadata.name="+dcName).FormatOutput("name"))
		return out
	}).WithTimeout(10*time.Minute).WithPolling(5*time.Second).Should(BeEmpty(), "CassandraDatacenter %s still exists", dcName)

	for _, resource := range []string{"pods", "persistentvolumeclaims", "statefulsets", "services", "poddisruptionbudgets"} {
		Eventually(func() string {
			out, _ := ns.Output(kubectl.Get(resource).WithLabel(selector).FormatOutput("name"))
			return out
		}).WithTimeout(5*time.Minute).WithPolling(5*time.Second).Should(BeEmpty(), "%s of %s left after deletion", resource, dcName)
	}

	secrets := &corev1.SecretList{}
	ns.getJson(secrets, kubectl.Get("secrets"))
	dcKey := types.NamespacedName{Namespace: ns.Namespace, Name: dcName}
	for _, secret := range secrets.Items {
		if annotation, found := secret.Annotations[watchedByAnnotation]; found {
			watchers := []types.NamespacedName{}
			Expect(json.Unmarshal([]byte(annotation), &watchers)).To(Succeed(), "annotation %s of secret %s", watchedByAnnotation, secret.Name)
			Expect(watchers).ToNot(ContainElement(dcKey), "secret %s is still watched by the deleted datacenter %s", secret.Name, dcName)
		}
	}
}

// AvailabilitySampler polls the pods of a datacenter in the background and records when more pods are unavailable
// at the same time than allowed.
type AvailabilitySampler struct {
	ns             NsWrapper
	dcName         string
	maxUnavailable int
	stop           chan struct{}
	done           chan struct{}

	mu          sync.Mutex
	violations  []string
	samples     int
	maxObserved int
	errors      int
}

// SampleAvailability starts polling the pods of the datacenter every 3 seconds. It fails the test in Stop() if more
// than one pod was unavailable (missing, terminating or not Ready) at the same time, not counting pods that were
// already unavailable when sampling started.
func (ns NsWrapper) SampleAvailability(dcName string) *AvailabilitySampler {
	return ns.SampleAvailabilityWithMaxUnavailable(dcName, 1)
}

func (ns NsWrapper) SampleAvailabilityWithMaxUnavailable(dcName string, maxUnavailable int) *AvailabilitySampler {
	ginkgo.By(fmt.Sprintf("sampling the availability of %s (at most %d pod(s) unavailable)", dcName, maxUnavailable))

	expected := map[string]bool{}
	for _, sts := range ns.GetDatacenterStatefulSets(dcName) {
		for i := 0; i < int(*sts.Spec.Replicas); i++ {
			expected[fmt.Sprintf("%s-%d", sts.Name, i)] = true
		}
	}
	for _, pod := range ns.GetDatacenterPods(dcName) {
		if !isPodReady(&pod) {
			delete(expected, pod.Name)
		}
	}

	s := &AvailabilitySampler{
		ns:             ns,
		dcName:         dcName,
		maxUnavailable: maxUnavailable,
		stop:           make(chan struct{}),
		done:           make(chan struct{}),
	}
	go s.run(expected)
	return s
}

func (s *AvailabilitySampler) run(expected map[string]bool) {
	defer close(s.done)
	defer ginkgo.GinkgoRecover()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		s.sample(expected)
		select {
		case <-s.stop:
			return
		case <-ticker.C:
		}
	}
}

func (s *AvailabilitySampler) sample(expected map[string]bool) {
	out, err := s.ns.Output(kubectl.Get("pods").WithLabel(dcSelector(s.dcName)).FormatOutput("json"))
	pods := &corev1.PodList{}
	if err == nil {
		err = json.Unmarshal([]byte(out), pods)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.errors++
		return
	}
	ready := map[string]bool{}
	for i := range pods.Items {
		ready[pods.Items[i].Name] = isPodReady(&pods.Items[i])
	}
	unavailable := []string{}
	for name := range expected {
		if !ready[name] {
			unavailable = append(unavailable, name)
		}
	}
	sort.Strings(unavailable)
	s.samples++
	if len(unavailable) > s.maxObserved {
		s.maxObserved = len(unavailable)
	}
	if len(unavailable) > s.maxUnavailable {
		s.violations = append(s.violations, fmt.Sprintf("%s: %v", time.Now().Format(time.RFC3339), unavailable))
	}
}

// Stop ends the sampling and fails the test if the availability was violated
func (s *AvailabilitySampler) Stop() {
	close(s.stop)
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	ginkgo.By(fmt.Sprintf("availability of %s: %d samples, at most %d pod(s) unavailable at once", s.dcName, s.samples, s.maxObserved))
	Expect(s.samples).To(BeNumerically(">", 0), "no successful availability samples for %s (%d errors)", s.dcName, s.errors)
	Expect(s.violations).To(BeEmpty(), "more than %d pod(s) of %s unavailable at the same time", s.maxUnavailable, s.dcName)
}
