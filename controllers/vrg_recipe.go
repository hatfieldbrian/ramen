// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	ramen "github.com/ramendr/ramen/api/v1alpha1"
	recipe "github.com/ramendr/recipe/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func captureWorkflowDefault(vrg ramen.VolumeReplicationGroup) []ramen.CaptureSpec {
	return []ramen.CaptureSpec{
		{
			OperationSpec: ramen.OperationSpec{
				ResourcesSpec: ramen.ResourcesSpec{
					IncludedNamespaces: []string{vrg.Namespace},
				},
			},
		},
	}
}

func recoverWorkflowDefault() []ramen.RecoverSpec { return []ramen.RecoverSpec{{}} }

func GetPVCSelector(ctx context.Context, reader client.Reader, vrg ramen.VolumeReplicationGroup,
	log logr.Logger,
) (ramen.PvcSelector, error) {
	err := recipeVolumesAndOptionallyWorkflowsGet(ctx, reader, &vrg, log,
		func(recipe.Recipe, *ramen.Recipe, ramen.VolumeReplicationGroup) error { return nil },
	)

	return vrg.Status.KubeObjectProtection.Recipe.PvcSelector, err
}

func recipeGet(ctx context.Context, reader client.Reader, vrg *ramen.VolumeReplicationGroup,
	log logr.Logger,
) error {
	return recipeVolumesAndOptionallyWorkflowsGet(ctx, reader, vrg, log, recipeWorkflowsGet)
}

func recipeVolumesAndOptionallyWorkflowsGet(ctx context.Context, reader client.Reader,
	vrg *ramen.VolumeReplicationGroup,
	log logr.Logger, workflowsGet func(recipe.Recipe, *ramen.Recipe, ramen.VolumeReplicationGroup) error,
) error {
	recipe1 := &vrg.Status.KubeObjectProtection.Recipe

	if vrg.Spec.KubeObjectProtection == nil {
		*recipe1 = ramen.Recipe{
			PvcSelector: pvcSelectorDefault(*vrg),
		}

		return nil
	}

	if vrg.Spec.KubeObjectProtection.RecipeRef == nil {
		*recipe1 = ramen.Recipe{
			PvcSelector:     pvcSelectorDefault(*vrg),
			CaptureWorkflow: captureWorkflowDefault(*vrg),
			RecoverWorkflow: recoverWorkflowDefault(),
		}

		return nil
	}

	recipe := recipe.Recipe{}
	if err := reader.Get(ctx, types.NamespacedName{
		Namespace: vrg.Spec.KubeObjectProtection.RecipeRef.Namespace,
		Name:      vrg.Spec.KubeObjectProtection.RecipeRef.Name,
	}, &recipe); err != nil {
		return errors.Wrap(err, "recipe get")
	}

	if err := RecipeParametersExpand(&recipe, vrg.Spec.KubeObjectProtection.RecipeParameters, log); err != nil {
		return errors.Wrap(err, "recipe parameters expand")
	}

	*recipe1 = ramen.Recipe{
		PvcSelector: pvcSelectorRecipeRefNonNil(recipe, *vrg),
	}

	return workflowsGet(recipe, recipe1, *vrg)
}

func recipeWorkflowsGet(recipe recipe.Recipe, recipe1 *ramen.Recipe, vrg ramen.VolumeReplicationGroup) error {
	var err error

	if recipe.Spec.CaptureWorkflow == nil {
		recipe1.CaptureWorkflow = captureWorkflowDefault(vrg)
	} else {
		recipe1.CaptureWorkflow, err = getCaptureGroups(recipe)
		if err != nil {
			return errors.Wrap(err, "Failed to get groups from capture workflow")
		}
	}

	if recipe.Spec.RecoverWorkflow == nil {
		recipe1.RecoverWorkflow = recoverWorkflowDefault()
	} else {
		recipe1.RecoverWorkflow, err = getRecoverGroups(recipe)
		if err != nil {
			return errors.Wrap(err, "Failed to get groups from recovery workflow")
		}
	}

	return err
}

func RecipeParametersExpand(recipe *recipe.Recipe, parameters map[string][]string,
	log logr.Logger,
) error {
	spec := &recipe.Spec
	log.Info("Recipe pre-expansion", "spec", *spec, "parameters", parameters)

	bytes, err := json.Marshal(*spec)
	if err != nil {
		return err
	}

	s1 := string(bytes)
	s2 := parametersExpand(s1, parameters)

	if err = json.Unmarshal([]byte(s2), spec); err != nil {
		return err
	}

	log.Info("Recipe post-expansion", "spec", *spec)

	return nil
}

func parametersExpand(s string, parameters map[string][]string) string {
	return os.Expand(s, func(key string) string {
		values := parameters[key]

		return strings.Join(values, `","`)
	})
}
