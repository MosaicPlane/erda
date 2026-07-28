// Copyright (c) 2026 MosaicPlane Authors.
// Licensed under the Apache License, Version 2.0.

package runtime

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/erda-project/erda/apistructs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRolloutUsesManualFirstBatchGate(t *testing.T) {
	target := progressiveTarget{serviceName: "api", workloadName: "api", namespace: "runtime-1"}
	rollout := buildRollout(42, target, apistructs.ProgressiveReleaseConfig{
		ServiceName: "api", Enabled: true, FirstBatchReplicas: 1, ObservationSeconds: 300,
	})
	steps, found, err := unstructured.NestedSlice(rollout.Object, "spec", "strategy", "canary", "steps")
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, steps, 2)
	first := steps[0].(map[string]interface{})
	assert.Equal(t, int64(1), first["replicas"])
	assert.Equal(t, map[string]interface{}{}, first["pause"])
	assert.Equal(t, "300", rollout.GetAnnotations()[progressiveObservationAnnotation])
}

func TestProgressiveStatusEnforcesObservationWindow(t *testing.T) {
	started := time.Date(2026, 7, 28, 8, 0, 0, 0, time.UTC)
	rollout := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"annotations": map[string]interface{}{
			progressiveObservationAnnotation: "300", progressiveFirstBatchAnnotation: "1",
		}},
		"status": map[string]interface{}{
			"phase": "Progressing",
			"canaryStatus": map[string]interface{}{
				"currentStepIndex": int64(1), "currentStepState": "StepPaused", "lastUpdateTime": started.Format(time.RFC3339),
			},
		},
	}}
	target := progressiveTarget{serviceName: "api", workloadName: "api", namespace: "runtime-1"}

	waiting := progressiveStatusFromRollout(target, rollout, started.Add(2*time.Minute))
	assert.False(t, waiting.CanApprove)
	assert.True(t, waiting.CanRollback)
	assert.Equal(t, int64(180), waiting.RemainingSeconds)

	ready := progressiveStatusFromRollout(target, rollout, started.Add(5*time.Minute))
	assert.True(t, ready.CanApprove)
	assert.Zero(t, ready.RemainingSeconds)
}

func TestStableReplicaSet(t *testing.T) {
	items := []appsv1.ReplicaSet{{ObjectMeta: metav1.ObjectMeta{
		Name: "api-abc123", Labels: map[string]string{appsv1.DefaultDeploymentUniqueLabelKey: "abc123"},
	}}}
	stable, err := stableReplicaSet(items, "abc123")
	require.NoError(t, err)
	assert.Equal(t, "api-abc123", stable.Name)
}

func TestProgressiveStatusRecognizesCompletedRollback(t *testing.T) {
	rollout := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"annotations": map[string]interface{}{}},
		"status": map[string]interface{}{
			"phase": "Healthy", "message": "Rollout progressing has been cancelled",
			"canaryStatus": map[string]interface{}{"currentStepIndex": int64(1), "currentStepState": "StepPaused"},
		},
	}}
	status := progressiveStatusFromRollout(progressiveTarget{serviceName: "api"}, rollout, time.Now())
	assert.Equal(t, "RollbackComplete", status.CurrentStepState)
	assert.False(t, status.CanApprove)
	assert.False(t, status.CanRollback)
}
