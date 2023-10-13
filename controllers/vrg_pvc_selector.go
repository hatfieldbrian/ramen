// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	ramen "github.com/ramendr/ramen/api/v1alpha1"
	recipe "github.com/ramendr/recipe/api/v1alpha1"
)

func pvcNamespaceNamesDefault(vrg ramen.VolumeReplicationGroup) []string {
	return []string{vrg.Namespace}
}

func pvcSelectorDefault(vrg ramen.VolumeReplicationGroup) ramen.PvcSelector {
	return ramen.PvcSelector{
		LabelSelector:  vrg.Spec.PVCSelector,
		NamespaceNames: pvcNamespaceNamesDefault(vrg),
	}
}

func pvcSelectorRecipeRefNonNil(recipe recipe.Recipe, vrg ramen.VolumeReplicationGroup) ramen.PvcSelector {
	if recipe.Spec.Volumes == nil {
		return pvcSelectorDefault(vrg)
	}

	var selector ramen.PvcSelector

	if recipe.Spec.Volumes.LabelSelector != nil {
		selector.LabelSelector = *recipe.Spec.Volumes.LabelSelector
	}

	if len(recipe.Spec.Volumes.IncludedNamespaces) > 0 {
		selector.NamespaceNames = recipe.Spec.Volumes.IncludedNamespaces
	} else {
		selector.NamespaceNames = pvcNamespaceNamesDefault(vrg)
	}

	return selector
}
