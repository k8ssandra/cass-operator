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
	policyv1 "k8s.io/api/policy/v1"
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
	allowedWarningActions map[string]bool
	skipRevisionCheck     bool
	skipRingCheck         bool
	skipQuietCheck        bool
}

type InvariantOption func(*invariantOptions)

// AllowWarningEvents allows Warning events with the given actions (the event constants of pkg/events) since the last
// checkpoint, for steps that break the datacenter on purpose.
func AllowWarningEvents(actions ...string) InvariantOption {
	return func(o *invariantOptions) {
		for _, action := range actions {
			o.allowedWarningActions[action] = true
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
	o := &invariantOptions{allowedWarningActions: map[string]bool{}}
	for _, opt := range opts {
		opt(o)
	}

	ginkgo.By(fmt.Sprintf("checking the invariants of datacenter %s", dcName))
	checkpoint, setCheckpoint := ns.eventCheckpoint(dcName)
	checkTime := time.Now()

	// Tasks the operator created itself (cleanup after a scale up) may still run on a Ready datacenter, but they must finish
	Eventually(func() []corev1.ObjectReference {
		return ns.GetDatacenter(dcName).Status.TrackedTasks
	}).WithTimeout(10*time.Minute).WithPolling(5*time.Second).Should(BeEmpty(), "status.trackedTasks of %s", dcName)

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
	Expect(dc.Status.NodeStatuses).To(HaveLen(len(pods)), "status.nodeStatuses of %s has entries for nodes that are gone", dcName)

	// Storage: one server-data PVC per node, no PVC of a removed node left behind
	pvcs := &corev1.PersistentVolumeClaimList{}
	ns.getJson(pvcs, kubectl.Get("persistentvolumeclaims").WithLabel(dcSelector(dcName)))
	serverData := []string{}
	for _, pvc := range pvcs.Items {
		if strings.HasPrefix(pvc.Name, "server-data-") {
			Expect(pvc.Status.Phase).To(Equal(corev1.ClaimBound), "PVC %s is not Bound", pvc.Name)
			serverData = append(serverData, pvc.Name)
		}
	}
	expectedPVCs := []string{}
	for i := range pods {
		expectedPVCs = append(expectedPVCs, "server-data-"+pods[i].Name)
	}
	Expect(serverData).To(ConsistOf(expectedPVCs), "server-data PVCs of %s", dcName)

	// StatefulSets
	for _, sts := range ns.GetDatacenterStatefulSets(dcName) {
		Expect(sts.Status.ObservedGeneration).To(Equal(sts.Generation), "StatefulSet %s status.observedGeneration", sts.Name)
		Expect(sts.Status.ReadyReplicas).To(Equal(*sts.Spec.Replicas), "StatefulSet %s readyReplicas", sts.Name)
		if !o.skipRevisionCheck {
			Expect(sts.Status.CurrentRevision).To(Equal(sts.Status.UpdateRevision), "StatefulSet %s is not on its update revision", sts.Name)
			Expect(sts.Status.UpdatedReplicas).To(Equal(*sts.Spec.Replicas), "StatefulSet %s updatedReplicas", sts.Name)
		}
	}

	// PodDisruptionBudget follows the size
	if dc.Spec.MaxUnavailable == nil {
		pdbs := &policyv1.PodDisruptionBudgetList{}
		ns.getJson(pdbs, kubectl.Get("poddisruptionbudgets").WithLabel(dcSelector(dcName)))
		Expect(pdbs.Items).To(HaveLen(1), "PodDisruptionBudgets of %s", dcName)
		Expect(pdbs.Items[0].Spec.MinAvailable).ToNot(BeNil(), "PodDisruptionBudget %s has no minAvailable", pdbs.Items[0].Name)
		Expect(pdbs.Items[0].Spec.MinAvailable.IntValue()).To(Equal(int(dc.Spec.Size)-1), "minAvailable of PodDisruptionBudget %s", pdbs.Items[0].Name)
	}

	// Ring, as every node sees it
	if !o.skipRingCheck {
		for i := range pods {
			ringHostIDs := []string{}
			for _, node := range ns.RetrieveStatusFromNodetool(pods[i].Name) {
				if node.Datacenter != dc.DatacenterName() {
					continue
				}
				Expect(node.Status+"/"+node.State).To(Equal("up/normal"), "node %s (%s) in nodetool status on %s", node.Address, node.HostId, pods[i].Name)
				ringHostIDs = append(ringHostIDs, node.HostId)
			}
			Expect(ringHostIDs).To(ConsistOf(hostIDs), "the ring of %s on %s does not match status.nodeStatuses", dcName, pods[i].Name)
		}
	}

	// Warning events
	warnings := ns.WarningEventsSince(dcName, checkpoint)
	unexpected := []string{}
	for _, event := range warnings {
		if !o.allowedWarningActions[EventAction(&event)] && !isStartupRequeue(&event) {
			unexpected = append(unexpected, fmt.Sprintf("%s: %s", EventAction(&event), EventText(&event)))
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

// EventAction returns the event constant (pkg/events) of an operator event. The operator's recorder stores it in the
// action of the event and the message in the reason, events of other components have the constant in the reason.
func EventAction(event *corev1.Event) string {
	if event.Action != "" {
		return event.Action
	}
	return event.Reason
}

// EventText returns the message of an operator event, which the operator's recorder stores in the reason
func EventText(event *corev1.Event) string {
	if event.Action == "" {
		return event.Message
	}
	return strings.TrimSpace(event.Reason + " " + event.Message)
}

// startupRequeueErrors are the errors with which the operator requeues while the pods of a new or scaled up rack are
// created and started: the StatefulSet controller has not created a pod yet, or a Ready pod has not been labelled
// Started yet. The operator reports them as ReconcileFailed Warning events in every test that creates or scales up a
// datacenter.
var startupRequeueErrors = []*regexp.Regexp{
	regexp.MustCompile(`^pod \S+: pod not found, most likely statefulset controller has not caught up yet$`),
	regexp.MustCompile(`^no pods were ready to be started in any rack$`),
	regexp.MustCompile(`^checks failed desired:\d+, ready:\d+, started:\d+$`),
}

func isStartupRequeue(event *corev1.Event) bool {
	if EventAction(event) != "ReconcileFailed" {
		return false
	}
	for _, re := range startupRequeueErrors {
		if re.MatchString(EventText(event)) {
			return true
		}
	}
	return false
}

// WarningEventsSince returns the Warning events of the CassandraDatacenter that happened after the given time
func (ns NsWrapper) WarningEventsSince(dcName string, since time.Time) []corev1.Event {
	return ns.EventsSince(dcName, corev1.EventTypeWarning, since)
}

// EventsSince returns the events of the given type of the CassandraDatacenter that happened after the given time
func (ns NsWrapper) EventsSince(dcName, eventType string, since time.Time) []corev1.Event {
	events := &corev1.EventList{}
	ns.getJson(events, kubectl.Get("events").
		WithFlag("field-selector", fmt.Sprintf("involvedObject.kind=CassandraDatacenter,involvedObject.name=%s,type=%s", dcName, eventType)))

	result := []corev1.Event{}
	for i := range events.Items {
		// Event timestamps have a resolution of a second
		if !eventTime(&events.Items[i]).Before(since.Truncate(time.Second)) {
			result = append(result, events.Items[i])
		}
	}
	return result
}

// WaitForWarningEvent waits for a Warning event of the CassandraDatacenter with the given action (an event constant
// of pkg/events) and a message matching the pattern, after the given time.
func (ns NsWrapper) WaitForWarningEvent(dcName string, since time.Time, action, messagePattern string, timeout time.Duration) {
	ginkgo.By(fmt.Sprintf("waiting for a %s Warning event for %s", action, dcName))
	re := regexp.MustCompile(messagePattern)
	Eventually(func() bool {
		for _, event := range ns.WarningEventsSince(dcName, since) {
			if EventAction(&event) == action && re.MatchString(EventText(&event)) {
				return true
			}
		}
		return false
	}).WithTimeout(timeout).WithPolling(5*time.Second).Should(BeTrue(), "no %s Warning event matching %q", action, messagePattern)
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

// ExpectStoppedDatacenterKeepsStorage checks a stopped datacenter: every StatefulSet is kept with 0 replicas and the
// PVCs of all nodes are kept.
func (ns NsWrapper) ExpectStoppedDatacenterKeepsStorage(dcName string, size int) {
	ginkgo.By(fmt.Sprintf("checking that the stopped datacenter %s keeps its StatefulSets and PVCs", dcName))
	statefulSets := ns.GetDatacenterStatefulSets(dcName)
	Expect(statefulSets).ToNot(BeEmpty(), "the StatefulSets of the stopped datacenter %s are gone", dcName)
	for _, sts := range statefulSets {
		Expect(*sts.Spec.Replicas).To(BeZero(), "StatefulSet %s of the stopped datacenter has replicas", sts.Name)
	}

	pvcs := &corev1.PersistentVolumeClaimList{}
	ns.getJson(pvcs, kubectl.Get("persistentvolumeclaims").WithLabel(dcSelector(dcName)))
	bound := 0
	for _, pvc := range pvcs.Items {
		if pvc.Status.Phase == corev1.ClaimBound && pvc.DeletionTimestamp == nil {
			bound++
		}
	}
	Expect(bound).To(Equal(size), "bound PVCs of the stopped datacenter %s", dcName)
}

// DecommissionReplicaSampler polls the StatefulSets of a datacenter while a pod is labelled Decommissioning
type DecommissionReplicaSampler struct {
	stop chan struct{}
	done chan struct{}

	mu             sync.Mutex
	decommissioned int
	violations     []string
}

// SampleDecommissionReplicas starts polling the datacenter every 2 seconds. Stop() fails the test if the StatefulSet
// replicas went below the given node count while the Decommissioning pod was still up in the ring: the pod must only
// be removed once Cassandra finished the decommission.
func SampleDecommissionReplicas(ns NsWrapper, dcName string, nodesBefore int) *DecommissionReplicaSampler {
	ginkgo.By(fmt.Sprintf("sampling the StatefulSet replicas of %s during the decommission", dcName))
	s := &DecommissionReplicaSampler{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(s.done)
		defer ginkgo.GinkgoRecover()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			s.sample(ns, dcName, nodesBefore)
			select {
			case <-s.stop:
				return
			case <-ticker.C:
			}
		}
	}()
	return s
}

func (s *DecommissionReplicaSampler) sample(ns NsWrapper, dcName string, nodesBefore int) {
	podsOut, podsErr := ns.Output(kubectl.Get("pods").WithLabel(dcSelector(dcName)).FormatOutput("json"))
	stsOut, stsErr := ns.Output(kubectl.Get("statefulsets").WithLabel(dcSelector(dcName)).FormatOutput("json"))
	if podsErr != nil || stsErr != nil {
		return
	}
	pods := &corev1.PodList{}
	sts := &appsv1.StatefulSetList{}
	if json.Unmarshal([]byte(podsOut), pods) != nil || json.Unmarshal([]byte(stsOut), sts) != nil {
		return
	}
	var decommissioning *corev1.Pod
	var other *corev1.Pod
	for i := range pods.Items {
		pod := &pods.Items[i]
		switch pod.Labels[nodeStateLabel] {
		case "Decommissioning":
			decommissioning = pod
		case "Started":
			if isPodReady(pod) {
				other = pod
			}
		}
	}
	if decommissioning == nil {
		return
	}
	replicas := 0
	for _, set := range sts.Items {
		replicas += int(*set.Spec.Replicas)
	}

	violation := ""
	if replicas < nodesBefore && other != nil {
		// The replicas may only go down once the node has left the ring
		out, err := ns.Output(kubectl.ExecOnPod(other.Name, "-c", "cassandra", "--", "nodetool", "status"))
		if err == nil {
			for _, node := range parseNodetoolStatus(out) {
				if node.Address == decommissioning.Status.PodIP && node.Status == "up" {
					violation = fmt.Sprintf("%s: %d replicas while %s (%s) is still %s in the ring",
						time.Now().Format(time.RFC3339), replicas, decommissioning.Name, node.Address, node.State)
				}
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.decommissioned++
	if violation != "" {
		s.violations = append(s.violations, violation)
	}
}

// Stop ends the sampling and fails the test if the replicas were reduced too early
func (s *DecommissionReplicaSampler) Stop() {
	close(s.stop)
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	Expect(s.decommissioned).To(BeNumerically(">", 0), "no sample saw a Decommissioning pod")
	Expect(s.violations).To(BeEmpty(), "StatefulSet replicas were reduced before the decommission finished")
}

// ExpectCassandraSetting waits until the setting in system_views.settings of the node matches the pattern, which
// shows that a configuration change is effective in Cassandra and not only rendered into the configuration files.
func (ns NsWrapper) ExpectCassandraSetting(podName, clusterName, setting, pattern string) {
	ginkgo.By(fmt.Sprintf("checking that %s matches %q in system_views.settings of %s", setting, pattern, podName))
	user, password := ns.RetrieveSuperuserCreds(clusterName)
	query := fmt.Sprintf("SELECT value FROM system_views.settings WHERE name = '%s'", setting)
	re := regexp.MustCompile(pattern)
	output := ""
	Eventually(func() bool {
		out, err := ns.Output(cqlsh(podName, query, user, password))
		output = out
		return err == nil && re.MatchString(out)
	}).WithTimeout(2*time.Minute).WithPolling(10*time.Second).Should(BeTrue(), func() string {
		return fmt.Sprintf("setting %s does not match %q: %s", setting, pattern, output)
	})
}
