// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"github.com/go-logr/logr"
	ramen "github.com/ramendr/ramen/api/v1alpha1"
	"github.com/ramendr/ramen/controllers/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func vrgConditionTrueAndCurrentTest(vrg ramen.VolumeReplicationGroup, conditionType string, log logr.Logger) bool {
	return util.ConditionTrueAndCurrentTest(vrg.GetGeneration(), vrg.Status.Conditions, conditionType, log)
}

func setVRGFinalSyncPrepared(vrg *ramen.VolumeReplicationGroup, message string) {
	setStatusCondition(&vrg.Status.Conditions, metav1.Condition{
		Type:               VRGConditionTypeFinalSyncPrepared,
		Reason:             VRGConditionReasonReady,
		ObservedGeneration: vrg.Generation,
		Status:             metav1.ConditionTrue,
		Message:            message,
	})
}

func setVRGFinalSyncComplete(vrg *ramen.VolumeReplicationGroup, message string) {
	setStatusCondition(&vrg.Status.Conditions, metav1.Condition{
		Type:               VRGConditionTypeFinalSyncComplete,
		Reason:             VRGConditionReasonReady,
		ObservedGeneration: vrg.Generation,
		Status:             metav1.ConditionTrue,
		Message:            message,
	})
}
