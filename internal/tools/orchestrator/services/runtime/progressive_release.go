// Copyright (c) 2021 Terminus, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package runtime

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrorsk8s "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/erda-project/erda/apistructs"
	"github.com/erda-project/erda/internal/pkg/user"
	executorutil "github.com/erda-project/erda/internal/tools/orchestrator/scheduler/executor/util"
	"github.com/erda-project/erda/pkg/k8sclient"
	"github.com/erda-project/erda/pkg/strutil"
)

const (
	progressiveManagedByAnnotation       = "mosaicplane.io/progressive-release"
	progressiveRuntimeIDAnnotation       = "mosaicplane.io/runtime-id"
	progressiveServiceAnnotation         = "mosaicplane.io/service-name"
	progressiveObservationAnnotation     = "mosaicplane.io/observation-seconds"
	progressiveFirstBatchAnnotation      = "mosaicplane.io/first-batch-replicas"
	defaultObservationSeconds        int = 300
)

var rolloutGVR = schema.GroupVersionResource{
	Group: "rollouts.kruise.io", Version: "v1beta1", Resource: "rollouts",
}

type progressiveTarget struct {
	serviceName  string
	workloadName string
	namespace    string
	clusterName  string
}

func newDynamicClient(clusterName string) (dynamic.Interface, error) {
	restConfig, err := k8sclient.GetRestConfig(clusterName)
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(restConfig)
}

func (r *Runtime) checkProgressivePermission(operator user.ID, runtimeID uint64, action string) error {
	rt, err := r.db.GetRuntime(runtimeID)
	if err != nil {
		return err
	}
	perm, err := r.bdl.CheckPermission(&apistructs.PermissionCheckRequest{
		UserID: operator.String(), Scope: apistructs.AppScope, ScopeID: rt.ApplicationID,
		Resource: "runtime-" + strutil.ToLower(rt.Workspace), Action: action,
	})
	if err != nil {
		return err
	}
	if !perm.Access {
		return fmt.Errorf("permission denied")
	}
	return nil
}

func (r *Runtime) progressiveTargets(runtimeID uint64, serviceName string) ([]progressiveTarget, error) {
	rt, err := r.db.GetRuntime(runtimeID)
	if err != nil {
		return nil, err
	}
	if rt.ScheduleName.Namespace == "" || rt.ScheduleName.Name == "" {
		return nil, fmt.Errorf("runtime has no scheduled service group")
	}
	sg, err := r.serviceGroupImpl.InspectServiceGroupWithTimeout(rt.ScheduleName.Namespace, rt.ScheduleName.Name)
	if err != nil {
		return nil, err
	}
	var targets []progressiveTarget
	for i := range sg.Services {
		service := &sg.Services[i]
		if serviceName != "" && service.Name != serviceName {
			continue
		}
		if service.WorkLoad != "" && !strings.EqualFold(service.WorkLoad, "deployment") {
			continue
		}
		if service.Namespace == "" {
			return nil, fmt.Errorf("service %s has no Kubernetes namespace", service.Name)
		}
		targets = append(targets, progressiveTarget{
			serviceName: service.Name, workloadName: executorutil.GetDeployName(service),
			namespace: service.Namespace, clusterName: rt.ClusterName,
		})
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("stateless service %q not found", serviceName)
	}
	return targets, nil
}

// GetProgressiveReleases returns rollout policy and live step state for all
// stateless services in a runtime.
func (r *Runtime) GetProgressiveReleases(ctx context.Context, operator user.ID, runtimeID uint64) ([]apistructs.ProgressiveReleaseStatus, error) {
	if err := r.checkProgressivePermission(operator, runtimeID, apistructs.GetAction); err != nil {
		return nil, err
	}
	targets, err := r.progressiveTargets(runtimeID, "")
	if err != nil {
		return nil, err
	}
	client, err := newDynamicClient(targets[0].clusterName)
	if err != nil {
		return nil, err
	}
	result := make([]apistructs.ProgressiveReleaseStatus, 0, len(targets))
	for _, target := range targets {
		rollout, err := client.Resource(rolloutGVR).Namespace(target.namespace).Get(ctx, target.workloadName, metav1.GetOptions{})
		if apierrorsk8s.IsNotFound(err) {
			result = append(result, disabledProgressiveStatus(target))
			continue
		}
		if err != nil {
			return nil, err
		}
		result = append(result, progressiveStatusFromRollout(target, rollout, time.Now()))
	}
	return result, nil
}

// ConfigureProgressiveRelease creates the guard before the next workload
// revision is deployed. A nil pause duration intentionally requires explicit
// approval; MosaicPlane enforces the minimum observation window.
func (r *Runtime) ConfigureProgressiveRelease(ctx context.Context, operator user.ID, runtimeID uint64, config apistructs.ProgressiveReleaseConfig) (*apistructs.ProgressiveReleaseStatus, error) {
	if err := r.checkProgressivePermission(operator, runtimeID, apistructs.OperateAction); err != nil {
		return nil, err
	}
	targets, err := r.progressiveTargets(runtimeID, config.ServiceName)
	if err != nil {
		return nil, err
	}
	target := targets[0]
	client, err := newDynamicClient(target.clusterName)
	if err != nil {
		return nil, err
	}
	resource := client.Resource(rolloutGVR).Namespace(target.namespace)
	existing, getErr := resource.Get(ctx, target.workloadName, metav1.GetOptions{})
	if !config.Enabled {
		if apierrorsk8s.IsNotFound(getErr) {
			status := disabledProgressiveStatus(target)
			return &status, nil
		}
		if getErr != nil {
			return nil, getErr
		}
		state, _, _ := unstructured.NestedString(existing.Object, "status", "canaryStatus", "currentStepState")
		phase, _, _ := unstructured.NestedString(existing.Object, "status", "phase")
		if phase != "Healthy" && state != "" && state != "Complete" && state != "Completed" {
			return nil, fmt.Errorf("cannot disable an active progressive release")
		}
		if err := resource.Delete(ctx, target.workloadName, metav1.DeleteOptions{}); err != nil {
			return nil, err
		}
		status := disabledProgressiveStatus(target)
		return &status, nil
	}
	if config.FirstBatchReplicas <= 0 {
		config.FirstBatchReplicas = 1
	}
	if config.ObservationSeconds <= 0 {
		config.ObservationSeconds = defaultObservationSeconds
	}
	rollout := buildRollout(runtimeID, target, config)
	if getErr == nil {
		rollout.SetResourceVersion(existing.GetResourceVersion())
		rollout, err = resource.Update(ctx, rollout, metav1.UpdateOptions{})
	} else if apierrorsk8s.IsNotFound(getErr) {
		rollout, err = resource.Create(ctx, rollout, metav1.CreateOptions{})
	} else {
		return nil, getErr
	}
	if err != nil {
		return nil, err
	}
	status := progressiveStatusFromRollout(target, rollout, time.Now())
	return &status, nil
}

func (r *Runtime) ApproveProgressiveRelease(ctx context.Context, operator user.ID, runtimeID uint64, serviceName string) (*apistructs.ProgressiveReleaseStatus, error) {
	if err := r.checkProgressivePermission(operator, runtimeID, apistructs.OperateAction); err != nil {
		return nil, err
	}
	targets, err := r.progressiveTargets(runtimeID, serviceName)
	if err != nil {
		return nil, err
	}
	target := targets[0]
	client, err := newDynamicClient(target.clusterName)
	if err != nil {
		return nil, err
	}
	resource := client.Resource(rolloutGVR).Namespace(target.namespace)
	rollout, err := resource.Get(ctx, target.workloadName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	status := progressiveStatusFromRollout(target, rollout, time.Now())
	if !status.CanApprove {
		if status.RemainingSeconds > 0 {
			return nil, fmt.Errorf("observation period has %d seconds remaining", status.RemainingSeconds)
		}
		return nil, fmt.Errorf("rollout is not waiting for approval")
	}
	if err := unstructured.SetNestedField(rollout.Object, "StepReady", "status", "canaryStatus", "currentStepState"); err != nil {
		return nil, err
	}
	rollout, err = resource.UpdateStatus(ctx, rollout, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	updated := progressiveStatusFromRollout(target, rollout, time.Now())
	return &updated, nil
}

// RollbackProgressiveRelease restores the stable ReplicaSet pod template.
// Kruise detects this as a workload rollback and performs the fast native
// rollback path documented by the project.
func (r *Runtime) RollbackProgressiveRelease(ctx context.Context, operator user.ID, runtimeID uint64, serviceName string) (*apistructs.ProgressiveReleaseStatus, error) {
	if err := r.checkProgressivePermission(operator, runtimeID, apistructs.OperateAction); err != nil {
		return nil, err
	}
	targets, err := r.progressiveTargets(runtimeID, serviceName)
	if err != nil {
		return nil, err
	}
	target := targets[0]
	client, err := newDynamicClient(target.clusterName)
	if err != nil {
		return nil, err
	}
	rollout, err := client.Resource(rolloutGVR).Namespace(target.namespace).Get(ctx, target.workloadName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	stableRevision, _, _ := unstructured.NestedString(rollout.Object, "status", "canaryStatus", "stableRevision")
	if stableRevision == "" {
		return nil, fmt.Errorf("rollout has no stable revision to restore")
	}
	restConfig, err := k8sclient.GetRestConfig(target.clusterName)
	if err != nil {
		return nil, err
	}
	kubeClient, err := k8sclient.NewForRestConfig(restConfig)
	if err != nil {
		return nil, err
	}
	deployments := kubeClient.ClientSet.AppsV1().Deployments(target.namespace)
	deployment, err := deployments.Get(ctx, target.workloadName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	selector := labels.Set(deployment.Spec.Selector.MatchLabels).AsSelector().String()
	replicaSets, err := kubeClient.ClientSet.AppsV1().ReplicaSets(target.namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	stable, err := stableReplicaSet(replicaSets.Items, stableRevision)
	if err != nil {
		return nil, err
	}
	deployment.Spec.Template = *stable.Spec.Template.DeepCopy()
	// The Deployment controller adds pod-template-hash to ReplicaSets. It is
	// not part of the original Deployment template and must not be copied back,
	// otherwise the restore is interpreted as another new revision.
	delete(deployment.Spec.Template.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
	if _, err := deployments.Update(ctx, deployment, metav1.UpdateOptions{}); err != nil {
		return nil, err
	}
	status := progressiveStatusFromRollout(target, rollout, time.Now())
	status.Message = "rollback requested"
	status.CanApprove = false
	return &status, nil
}

func buildRollout(runtimeID uint64, target progressiveTarget, config apistructs.ProgressiveReleaseConfig) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rollouts.kruise.io/v1beta1",
		"kind":       "Rollout",
		"metadata": map[string]interface{}{
			"name": target.workloadName, "namespace": target.namespace,
			"annotations": map[string]interface{}{
				progressiveManagedByAnnotation:   "true",
				progressiveRuntimeIDAnnotation:   strconv.FormatUint(runtimeID, 10),
				progressiveServiceAnnotation:     target.serviceName,
				progressiveObservationAnnotation: strconv.Itoa(config.ObservationSeconds),
				progressiveFirstBatchAnnotation:  strconv.Itoa(config.FirstBatchReplicas),
			},
		},
		"spec": map[string]interface{}{
			"workloadRef": map[string]interface{}{
				"apiVersion": "apps/v1", "kind": "Deployment", "name": target.workloadName,
			},
			"strategy": map[string]interface{}{
				"canary": map[string]interface{}{
					"enableExtraWorkloadForCanary": false,
					"steps": []interface{}{
						map[string]interface{}{"replicas": int64(config.FirstBatchReplicas), "pause": map[string]interface{}{}},
						map[string]interface{}{"replicas": "100%"},
					},
				},
			},
		},
	}}
}

func disabledProgressiveStatus(target progressiveTarget) apistructs.ProgressiveReleaseStatus {
	return apistructs.ProgressiveReleaseStatus{
		ServiceName: target.serviceName, WorkloadName: target.workloadName,
		Namespace: target.namespace, Enabled: false,
	}
}

func progressiveStatusFromRollout(target progressiveTarget, rollout *unstructured.Unstructured, now time.Time) apistructs.ProgressiveReleaseStatus {
	annotations := rollout.GetAnnotations()
	observationSeconds := parseInt64(annotations[progressiveObservationAnnotation], defaultObservationSeconds)
	firstBatch := parseInt64(annotations[progressiveFirstBatchAnnotation], 1)
	phase, _, _ := unstructured.NestedString(rollout.Object, "status", "phase")
	message, _, _ := unstructured.NestedString(rollout.Object, "status", "message")
	step, _, _ := unstructured.NestedInt64(rollout.Object, "status", "canaryStatus", "currentStepIndex")
	state, _, _ := unstructured.NestedString(rollout.Object, "status", "canaryStatus", "currentStepState")
	lastUpdate, _, _ := unstructured.NestedString(rollout.Object, "status", "canaryStatus", "lastUpdateTime")
	status := apistructs.ProgressiveReleaseStatus{
		ServiceName: target.serviceName, WorkloadName: target.workloadName, Namespace: target.namespace,
		Enabled: true, Phase: phase, Message: message, CurrentStep: step, TotalSteps: 2,
		CurrentStepState: state, FirstBatchReplicas: firstBatch, ObservationSeconds: observationSeconds,
	}
	if phase == "Healthy" && strings.Contains(strings.ToLower(message), "cancel") {
		status.CurrentStepState = "RollbackComplete"
		return status
	}
	if state == "StepPaused" && step == 1 {
		status.CanRollback = true
		if started, err := time.Parse(time.RFC3339, lastUpdate); err == nil {
			started = started.UTC()
			ends := started.Add(time.Duration(observationSeconds) * time.Second)
			status.ObservationStartedAt = &started
			status.ObservationEndsAt = &ends
			remainingDuration := ends.Sub(now.UTC())
			if remainingDuration > 0 {
				status.RemainingSeconds = int64((remainingDuration + time.Second - 1) / time.Second)
			} else {
				status.CanApprove = true
			}
		}
	}
	return status
}

func parseInt64(value string, fallback int) int64 {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return int64(fallback)
	}
	return parsed
}

func stableReplicaSet(items []appsv1.ReplicaSet, stableRevision string) (*appsv1.ReplicaSet, error) {
	for i := range items {
		if items[i].Labels[appsv1.DefaultDeploymentUniqueLabelKey] == stableRevision || strings.HasSuffix(items[i].Name, stableRevision) {
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("stable ReplicaSet revision %s not found", stableRevision)
}
