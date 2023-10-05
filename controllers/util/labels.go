// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"golang.org/x/exp/maps" // TODO replace with "maps" in Go 1.21+
)

const (
	labelOwnerNamespaceName = "ramendr.openshift.io/owner-namespace-name"
	labelOwnerName          = "ramendr.openshift.io/owner-name"

	MModesLabel = "ramendr.openshift.io/maintenancemodes"
)

type Labels map[string]string

func OwnerLabels(ownerNamespaceName, ownerName string) Labels {
	return Labels{
		labelOwnerNamespaceName: ownerNamespaceName,
		labelOwnerName:          ownerName,
	}
}

func OwnerNamespaceNameAndName(labels Labels) (string, string, bool) {
	ownerNamespaceName, ok1 := labels[labelOwnerNamespaceName]
	ownerName, ok2 := labels[labelOwnerName]

	return ownerNamespaceName, ownerName, ok1 && ok2
}

func AddLabels(toAdd, existing Labels) (Labels, bool) {
	if existing == nil {
		return toAdd, true
	}

	length := len(existing)
	maps.Copy(toAdd, existing)

	return existing, length != len(existing)
}
