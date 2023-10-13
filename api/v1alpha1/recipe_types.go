// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Recipe struct {
	PvcSelector     PvcSelector   `json:"pvcSelector,omitempty"`
	CaptureWorkflow []CaptureSpec `json:"captureWorkflow,omitempty"`
	RecoverWorkflow []RecoverSpec `json:"recoverWorkflow,omitempty"`
}

type PvcSelector struct {
	LabelSelector  metav1.LabelSelector `json:"labelSelector,omitempty"`
	NamespaceNames []string             `json:"namespaceNames,omitempty"`
}

type CaptureSpec struct {
	//+optional
	Name          string `json:"name,omitempty"`
	OperationSpec `json:",inline"`
}

type RecoverSpec struct {
	//+optional
	BackupName    string `json:"backupName,omitempty"`
	OperationSpec `json:",inline"`
	//+optional
	NamespaceMapping map[string]string `json:"namespaceMapping,omitempty"`
	//+optional
	RestoreStatus *velero.RestoreStatusSpec `json:"restoreStatus,omitempty"`
	//+optional
	ExistingResourcePolicy velero.PolicyType `json:"existingResourcePolicy,omitempty"`
}

type OperationSpec struct {
	ResourcesSpec `json:",inline"`
	//+optional
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`
	//+optional
	OrLabelSelectors []*metav1.LabelSelector `json:"orLabelSelectors,omitempty"`
	//+optional
	IncludeClusterResources *bool `json:"includeClusterResources,omitempty"`
	//+optional
	Hooks []HookSpec `json:"hooks,omitempty"`
}

type ResourcesSpec struct {
	//+optional
	IncludedNamespaces []string `json:"includedNamespaces,omitempty"`
	//+optional
	IncludedResources []string `json:"includedResources,omitempty"`
	//+optional
	ExcludedResources []string `json:"excludedResources,omitempty"`
}

type HookSpec struct {
	Name    string   `json:"name,omitempty"`
	Type    string   `json:"type,omitempty"`
	Command []string `json:"command,omitempty"`
	//+optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`
	//+optional
	Container *string `json:"container,omitempty"`
	//+optional
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`
}
