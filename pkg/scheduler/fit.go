package scheduler

import (
	"context"
	"errors"

	"k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	plfeature "k8s.io/kubernetes/pkg/scheduler/framework/plugins/feature"

	// "k8s.io/kubernetes/pkg/scheduler/framework/plugins/interpodaffinity"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodeaffinity"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/noderesources"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/nodeunschedulable"
	"k8s.io/kubernetes/pkg/scheduler/framework/plugins/tainttoleration"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func createNodeInfos(ctx context.Context, cli client.Client) ([]*framework.NodeInfo, error) {
	nodes := corev1.NodeList{}
	if err := cli.List(ctx, &nodes); err != nil {
		return nil, err
	}

	nodeInfos := make([]*framework.NodeInfo, 0, len(nodes.Items))

	for _, node := range nodes.Items {
		nodeInfo := framework.NewNodeInfo()
		nodeInfo.SetNode(&node)
		pods := &corev1.PodList{}
		if err := cli.List(ctx, pods, client.MatchingFields{"spec.nodeName": nodeInfo.Node().Name}); err != nil {
			return nil, err
		}

		for _, pod := range pods.Items {
			nodeInfo.AddPod(&pod)
		}

		nodeInfos = append(nodeInfos, nodeInfo)
	}

	return nodeInfos, nil
}

// WillTheyFit checks if the proposed pods will be schedulable in the cluster. The ProposedPods must have their
// labels, tolerations, affinities and resource limits set
func WillTheyFit(ctx context.Context, cli client.Client, proposedPods []*corev1.Pod) error {
	usableNodes, err := createNodeInfos(ctx, cli)
	if err != nil {
		return err
	}

	state := framework.NewCycleState()

	schedulablePlugin, err := nodeunschedulable.New(ctx, nil, nil)
	if err != nil {
		return err
	}

	noderesourcesPlugin, err := noderesources.NewFit(ctx, &config.NodeResourcesFitArgs{ScoringStrategy: defaultScoringStrategy}, nil, plfeature.Features{})
	if err != nil {
		return err
	}

	nodeaffinityPlugin, err := nodeaffinity.New(ctx, &config.NodeAffinityArgs{}, nil)
	if err != nil {
		return err
	}

	// interpodaffinityPlugin, err := interpodaffinity.New(ctx, &config.InterPodAffinityArgs{}, snapshot)
	// if err != nil {
	// 	return err
	// }

	tainttolerationPlugin, err := tainttoleration.New(ctx, nil, nil)
	if err != nil {
		return err
	}

	plugins := []framework.FilterPlugin{
		noderesourcesPlugin.(framework.FilterPlugin),
		schedulablePlugin.(framework.FilterPlugin),
		nodeaffinityPlugin.(framework.FilterPlugin),
		// interpodaffinityPlugin.(framework.FilterPlugin),
		tainttolerationPlugin.(framework.FilterPlugin),
	}

NextPod:
	for _, pod := range proposedPods {
	NextNode:
		for _, node := range usableNodes {
			podInfo, err := framework.NewPodInfo(pod)
			if err != nil {
				return err
			}

			for _, plugin := range plugins {
				if prefilterPlugin, ok := plugin.(framework.PreFilterPlugin); ok {
					prefilterPlugin.PreFilter(ctx, state, podInfo.Pod)
				}
				status := plugin.Filter(ctx, state, podInfo.Pod, node)
				if status.Code() != framework.Success {
					continue NextNode
				}
			}

			node.AddPod(pod)
			continue NextPod
		}
		// Pod was never added to any node
		return errors.New(framework.Unschedulable.String())
	}
	return nil
}

var defaultScoringStrategy = &config.ScoringStrategy{
	Type: config.LeastAllocated,
	Resources: []config.ResourceSpec{
		{Name: "cpu", Weight: 1},
		{Name: "memory", Weight: 1},
	},
}
