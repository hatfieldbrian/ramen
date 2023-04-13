// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	ramen "github.com/ramendr/ramen/api/v1alpha1"
	"github.com/ramendr/ramen/controllers/kubeobjects"
	"github.com/ramendr/ramen/controllers/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	Recipe "github.com/ramendr/recipe/api/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func kubeObjectsCaptureInterval(kubeObjectProtectionSpec *ramen.KubeObjectProtectionSpec) time.Duration {
	if kubeObjectProtectionSpec.CaptureInterval == nil {
		return ramen.KubeObjectProtectionCaptureIntervalDefault
	}

	return kubeObjectProtectionSpec.CaptureInterval.Duration
}

func kubeObjectsCapturePathNameAndNamePrefix(namespaceName, vrgName string, captureNumber int64) (string, string) {
	const numberBase = 10
	number := strconv.FormatInt(captureNumber, numberBase)

	return s3PathNamePrefix(namespaceName, vrgName) + "kube-objects/" + number + "/",
		// TODO fix: may exceed name capacity
		namespaceName + "--" + vrgName + "--" + number
}

func kubeObjectsCaptureName(prefix, groupName, s3ProfileName string) string {
	return prefix + "--" + groupName + "--" + s3ProfileName
}

func kubeObjectsRecoverNamePrefix(vrgNamespaceName, vrgName string) string {
	return vrgNamespaceName + "--" + vrgName
}

func kubeObjectsRecoverName(prefix string, groupNumber int) string {
	return prefix + "--" + strconv.Itoa(groupNumber)
}

func (v *VRGInstance) kubeObjectsProtectPrimary(result *ctrl.Result, s3StoreAccessors []s3StoreAccessor) {
	v.kubeObjectsProtect(result, s3StoreAccessors, kubeObjectsCaptureStartConditionallyPrimary)
}

func (v *VRGInstance) kubeObjectsProtectSecondary(result *ctrl.Result, s3StoreAccessors []s3StoreAccessor) {
	v.kubeObjectsProtect(result, s3StoreAccessors, kubeObjectsCaptureStartConditionallySecondary)
}

type captureStartConditionally func(
	*VRGInstance, *ctrl.Result, int64, time.Duration, time.Duration, func(),
)

func (v *VRGInstance) kubeObjectsProtect(
	result *ctrl.Result,
	s3StoreAccessors []s3StoreAccessor,
	captureStartConditionally captureStartConditionally,
) {
	if v.kubeObjectProtectionDisabled("capture") {
		return
	}

	if len(s3StoreAccessors) == 0 {
		result.Requeue = true // TODO remove; watch config map instead

		return
	}

	vrg := v.instance
	status := &vrg.Status.KubeObjectProtection

	captureToRecoverFrom := status.CaptureToRecoverFrom
	if captureToRecoverFrom == nil {
		v.log.Info("Kube objects capture-to-recover-from nil")

		captureToRecoverFrom = &ramen.KubeObjectsCaptureIdentifier{}
	}

	v.kubeObjectsCaptureStartOrResumeOrDelay(result, s3StoreAccessors, captureStartConditionally, captureToRecoverFrom)
}

func (v *VRGInstance) kubeObjectsCaptureStartOrResumeOrDelay(
	result *ctrl.Result, s3StoreAccessors []s3StoreAccessor,
	captureStartConditionally captureStartConditionally,
	captureToRecoverFrom *ramen.KubeObjectsCaptureIdentifier,
) {
	veleroNamespaceName := v.veleroNamespaceName()
	vrg := v.instance
	interval := kubeObjectsCaptureInterval(vrg.Spec.KubeObjectProtection)
	number := 1 - captureToRecoverFrom.Number
	generation := vrg.GetGeneration()
	log := v.log.WithValues("number", number, "generation", generation)
	pathName, namePrefix := kubeObjectsCapturePathNameAndNamePrefix(vrg.Namespace, vrg.Name, number)
	labels := util.OwnerLabels(vrg.Namespace, vrg.Name)
	captureStartOrResume := func(generation int64) {
		v.kubeObjectsCaptureStartOrResume(result, s3StoreAccessors, captureStartConditionally,
			number, pathName, namePrefix, veleroNamespaceName, interval, labels, generation)
	}

	requests, err := v.reconciler.kubeObjects.ProtectRequestsGet(
		v.ctx, v.reconciler.APIReader, veleroNamespaceName, labels)
	if err != nil {
		log.Error(err, "Kube objects capture in-progress query error")
		v.kubeObjectsCaptureFailed(err.Error())

		result.Requeue = true

		return
	}

	if count := requests.Count(); count > 0 {
		log.Info("Kube objects capture resume")
		captureStartOrResume(requests.Get(0).Object().GetGeneration())

		return
	}

	captureStartConditionally(
		v, result, captureToRecoverFrom.StartGeneration, time.Since(captureToRecoverFrom.StartTime.Time), interval,
		func() {
			if v.kubeObjectsCaptureDelete(result, s3StoreAccessors, number, pathName); result.Requeue {
				return
			}

			log.Info("Kube objects capture start")
			captureStartOrResume(generation)
		},
	)
}

func kubeObjectsCaptureStartConditionallySecondary(
	v *VRGInstance, result *ctrl.Result,
	captureStartGeneration int64, captureStartTimeSince, captureStartInterval time.Duration,
	captureStart func(),
) {
	if generation := v.instance.Generation; captureStartGeneration == generation {
		v.log.Info("Kube objects capture for relocate complete")

		return
	}

	v.kubeObjectsFinalCapturePending()
	captureStart()
}

func kubeObjectsCaptureStartConditionallyPrimary(
	v *VRGInstance, result *ctrl.Result,
	captureStartGeneration int64, captureStartTimeSince, captureStartInterval time.Duration,
	captureStart func(),
) {
	if delay := captureStartInterval - captureStartTimeSince; delay > 0 {
		v.log.Info("Kube objects capture start delay", "delay", delay, "interval", captureStartInterval)
		delaySetIfLess(result, delay, v.log)

		return
	}

	captureStart()
}

func (v *VRGInstance) kubeObjectsCaptureDelete(
	result *ctrl.Result, s3StoreAccessors []s3StoreAccessor, captureNumber int64, pathName string,
) {
	pathName += v.reconciler.kubeObjects.ProtectsPath()

	// current s3 profiles may differ from those at capture time
	for _, s3StoreAccessor := range s3StoreAccessors {
		if err := s3StoreAccessor.ObjectStorer.DeleteObjects(pathName); err != nil {
			v.log.Error(err, "Kube objects capture s3 objects delete error",
				"number", captureNumber,
				"profile", s3StoreAccessor.profileName,
			)
			v.kubeObjectsCaptureFailed(err.Error())

			result.Requeue = true

			return
		}
	}
}

const (
	vrgGenerationKey            = "ramendr.openshift.io/vrg-generation"
	vrgGenerationNumberBase     = 10
	vrgGenerationNumberBitCount = 64
)

func (v *VRGInstance) kubeObjectsCaptureStartOrResume(
	result *ctrl.Result, s3StoreAccessors []s3StoreAccessor, captureStartConditionally captureStartConditionally,
	captureNumber int64, pathName, namePrefix, veleroNamespaceName string, interval time.Duration,
	labels map[string]string, generation int64,
) {
	vrg := v.instance
	groups := v.getCaptureGroups()
	requests := make([]kubeobjects.ProtectRequest, len(groups)*len(s3StoreAccessors))
	requestsProcessedCount := 0
	requestsCompletedCount := 0
	annotations := map[string]string{vrgGenerationKey: strconv.FormatInt(generation, vrgGenerationNumberBase)}
	log := v.log.WithValues("number", captureNumber)

	for groupNumber, captureGroup := range groups {
		log1 := log.WithValues("group", groupNumber, "name", captureGroup.Name)

		for _, s3StoreAccessor := range s3StoreAccessors {
			log2 := log.WithValues("profile", s3StoreAccessor.profileName)
			request, err := v.reconciler.kubeObjects.ProtectRequestCreate(
				v.ctx, v.reconciler.Client, v.reconciler.APIReader, v.log,
				s3StoreAccessor.url, s3StoreAccessor.bucketName, s3StoreAccessor.regionName,
				pathName,
				s3StoreAccessor.veleroNamespaceSecretKeyRef,
				vrg.Namespace,
				captureGroup.Spec,
				veleroNamespaceName, kubeObjectsCaptureName(namePrefix, captureGroup.Name, s3StoreAccessor.profileName),
				labels, annotations)
			requests[requestsProcessedCount] = request
			requestsProcessedCount++

			if err == nil {
				log2.Info("Kube objects group captured", "start", request.StartTime(), "end", request.EndTime())
				requestsCompletedCount++

				continue
			}

			if errors.Is(err, kubeobjects.RequestProcessingError{}) {
				log2.Info("Kube objects group capturing", "state", err.Error())

				continue
			}

			log2.Error(err, "Kube objects group capture error")
			v.kubeObjectsCaptureFailed(err.Error())

			result.Requeue = true

			return
		}

		if requestsCompletedCount < requestsProcessedCount {
			log1.Info("Kube objects group capturing", "complete", requestsCompletedCount, "total", requestsProcessedCount)

			return
		}
	}

	v.kubeObjectsCaptureComplete(result, captureStartConditionally,
		captureNumber, veleroNamespaceName, interval, labels, requests[0].StartTime(),
		requests[0].Object().GetAnnotations())
}

func (v *VRGInstance) kubeObjectsCaptureComplete(
	result *ctrl.Result,
	captureStartConditionally captureStartConditionally,
	captureNumber int64, veleroNamespaceName string, interval time.Duration,
	labels map[string]string, startTime metav1.Time, annotations map[string]string,
) {
	vrg := v.instance
	status := &vrg.Status.KubeObjectProtection

	if err := v.reconciler.kubeObjects.ProtectRequestsDelete(
		v.ctx, v.reconciler.Client, veleroNamespaceName, labels,
	); err != nil {
		v.log.Error(err, "Kube objects capture requests delete error", "number", captureNumber)
		v.kubeObjectsCaptureFailed(err.Error())

		result.Requeue = true

		return
	}

	startGeneration, err := strconv.ParseInt(
		annotations[vrgGenerationKey], vrgGenerationNumberBase, vrgGenerationNumberBitCount)
	if err != nil {
		v.log.Error(err, "Kube objects capture generation string to int64 conversion error")
	}

	status.CaptureToRecoverFrom = &ramen.KubeObjectsCaptureIdentifier{
		Number: captureNumber, StartTime: startTime,
		StartGeneration: startGeneration,
	}
	v.kubeObjectsProtected = newVRGClusterDataProtectedCondition(vrg.Generation, clusterDataProtectedTrueMessage)
	captureStartTimeSince := time.Since(startTime.Time)
	v.log.Info("Kube objects captured", "recovery point", status.CaptureToRecoverFrom, "duration", captureStartTimeSince)
	captureStartConditionally(
		v, result, startGeneration, captureStartTimeSince, interval,
		func() {
			v.log.Info("Kube objects capture schedule to run immediately")
			delaySetMinimum(result)
		},
	)
}

func (v *VRGInstance) kubeObjectsCaptureFailed(message string) {
	if v.instance.Status.KubeObjectProtection.CaptureToRecoverFrom != nil {
		// TODO && time.Since(CaptureToRecoverFrom.StartTime) < Spec.KubeObjectProtection.CaptureInterval * 2 or 3
		return
	}

	v.kubeObjectsProtected = newVRGClusterDataUnprotectedCondition(v.instance.Generation, message)
}

func (v *VRGInstance) kubeObjectsFinalCapturePending() {
	v.kubeObjectsProtected = newVRGClusterDataUnprotectedCondition(
		v.instance.Generation,
		"Kube objects capture for relocate pending",
	)
}

func RecipeInfoExistsOnVRG(vrgInstance ramen.VolumeReplicationGroup) bool {
	return vrgInstance.Spec.KubeObjectProtection != nil &&
		vrgInstance.Spec.KubeObjectProtection.RecipeRef != nil &&
		vrgInstance.Spec.KubeObjectProtection.RecipeRef.Name != nil
}

func VolumeGroupNameExistsInWorkflow(vrgInstance ramen.VolumeReplicationGroup) bool {
	return RecipeInfoExistsOnVRG(vrgInstance) &&
		vrgInstance.Spec.KubeObjectProtection.RecipeRef.VolumeGroupName != nil
}

func GetLabelSelectorFromRecipeVolumeGroupWithName(name string, recipe *Recipe.Recipe) (metav1.LabelSelector, error) {
	labelSelector := &metav1.LabelSelector{} // init

	for _, group := range recipe.Spec.Groups {
		if group.Name == name {
			labelSelector, err := getLabelSelectorFromString(group.LabelSelector)

			return *labelSelector, err
		}
	}

	return *labelSelector, k8serrors.NewNotFound(schema.GroupResource{Resource: "Recipe.Spec.Group.Name"}, name)
}

func (v *VRGInstance) getCaptureGroups() []kubeobjects.CaptureSpec {
	if RecipeInfoExistsOnVRG(*v.instance) &&
		v.instance.Spec.KubeObjectProtection.RecipeRef.CaptureWorkflowName != nil {
		v.log.Info(fmt.Sprintf("getCaptureGroups found captureName '%s'",
			*v.instance.Spec.KubeObjectProtection.RecipeRef.CaptureWorkflowName))

		return v.getCaptureGroupsFromRecipe()
	}

	return []kubeobjects.CaptureSpec{{}}
}

func (v VRGInstance) getNameAndNamespaceString() string {
	return fmt.Sprintf("VRG Name: %s, Namespace: %s", v.instance.ObjectMeta.Name, v.instance.Namespace)
}

func (v *VRGInstance) getCaptureGroupsFromRecipe() []kubeobjects.CaptureSpec {
	workflow, recipe, err := v.getWorkflowAndRecipeFromName(*v.instance.Spec.KubeObjectProtection.RecipeRef.Name,
		*v.instance.Spec.KubeObjectProtection.RecipeRef.CaptureWorkflowName)
	if err != nil {
		v.log.Error(err, "failed to get Workflow from Recipe.", "vrgInfo", v.getNameAndNamespaceString())
	}

	groups, err := v.getCaptureGroupsFromWorkflow(*recipe, *workflow)
	if err != nil {
		v.log.Error(err, "failed to get Capture Groups from Workflow.", "vrgInfo", v.getNameAndNamespaceString())
	}

	v.log.Info(fmt.Sprintf("successfully found groups for '%s' capture spec on %s",
		*v.instance.Spec.KubeObjectProtection.RecipeRef.CaptureWorkflowName,
		v.getNameAndNamespaceString()))

	return groups
}

func (v *VRGInstance) getRecoverGroups() []kubeobjects.RecoverSpec {
	if RecipeInfoExistsOnVRG(*v.instance) &&
		v.instance.Spec.KubeObjectProtection.RecipeRef.RecoverWorkflowName != nil {
		v.log.Info(fmt.Sprintf("getRecoverGroups() found Recover Name '%s'",
			*v.instance.Spec.KubeObjectProtection.RecipeRef.RecoverWorkflowName))

		return v.getRecoverGroupsFromRecipe()
	}

	return []kubeobjects.RecoverSpec{{}}
}

func (v *VRGInstance) getRecoverGroupsFromRecipe() []kubeobjects.RecoverSpec {
	workflow, recipe, err := v.getWorkflowAndRecipeFromName(*v.instance.Spec.KubeObjectProtection.RecipeRef.Name,
		*v.instance.Spec.KubeObjectProtection.RecipeRef.RecoverWorkflowName)
	if err != nil {
		v.log.Error(err, "failed to get Workflow and Recipe from Name.", "vrgInfo",
			v.getNameAndNamespaceString())
	}

	groups, err := v.getRestoreGroupsFromWorkflow(*recipe, *workflow)
	if err != nil {
		v.log.Error(err, "failed to get Restore Groups from Workflow.", "vrgInfo",
			v.getNameAndNamespaceString())
	}

	v.log.Info(fmt.Sprintf("getRecoverGroupsFromRecipe() successfully found groups for recover spec. '%s' on %s",
		*v.instance.Spec.KubeObjectProtection.RecipeRef.RecoverWorkflowName, v.getNameAndNamespaceString()))

	return groups
}

func (v *VRGInstance) kubeObjectsRecover(result *ctrl.Result,
	s3ProfileName string, s3StoreProfile ramen.S3StoreProfile, objectStorer ObjectStorer,
) error {
	vrg := v.instance

	if v.kubeObjectProtectionDisabled("recovery") {
		return nil
	}

	sourceVrgNamespaceName := vrg.Namespace
	sourceVrgName := vrg.Name
	sourcePathNamePrefix := s3PathNamePrefix(sourceVrgNamespaceName, sourceVrgName)

	sourceVrg := &ramen.VolumeReplicationGroup{}
	if err := vrgObjectDownload(objectStorer, sourcePathNamePrefix, sourceVrg); err != nil {
		v.log.Error(err, "Kube objects capture-to-recover-from identifier get error")

		return nil
	}

	capture := sourceVrg.Status.KubeObjectProtection.CaptureToRecoverFrom
	if capture == nil {
		v.log.Info("Kube objects capture-to-recover-from identifier nil")

		return nil
	}

	vrg.Status.KubeObjectProtection.CaptureToRecoverFrom = capture

	return v.kubeObjectsRecoveryStartOrResume(
		result,
		s3StoreAccessor{
			objectStorer,
			s3ProfileName,
			s3StoreProfile.S3CompatibleEndpoint,
			s3StoreProfile.S3Bucket,
			s3StoreProfile.S3Region,
			s3StoreProfile.VeleroNamespaceSecretKeyRef,
		},
		sourceVrgNamespaceName,
		sourceVrgName,
		capture,
	)
}

func (v *VRGInstance) createRecoverOrProtectRequest(
	s3StoreAccessor s3StoreAccessor,
	sourceVrgNamespaceName, sourceVrgName string,
	capture *ramen.KubeObjectsCaptureIdentifier,
	groupNumber int,
	recoverGroup kubeobjects.RecoverSpec,
	veleroNamespaceName string,
	labels map[string]string,
	annotations map[string]string,
) (kubeobjects.Request, error) {
	vrg := v.instance
	capturePathName, captureNamePrefix := kubeObjectsCapturePathNameAndNamePrefix(
		sourceVrgNamespaceName, sourceVrgName, capture.Number)
	recoverNamePrefix := kubeObjectsRecoverNamePrefix(vrg.Namespace, vrg.Name)

	var request kubeobjects.Request

	var err error

	if recoverGroup.BackupName == ramen.ReservedBackupName {
		status := &vrg.Status.KubeObjectProtection
		captureToRecoverFrom := status.CaptureToRecoverFrom
		backupSequenceNumber := 1 - captureToRecoverFrom.Number // is this a good way to do this?
		backupName := fmt.Sprintf("%s-restore-%d", recoverGroup.BackupName, groupNumber)
		v.log.Info(fmt.Sprintf("backup: %s, captureToRecoverFrom: %d", backupName, captureToRecoverFrom.Number))

		pathName, namePrefix := kubeObjectsCapturePathNameAndNamePrefix(vrg.Namespace, vrg.Name, backupSequenceNumber)
		request, err = v.reconciler.kubeObjects.ProtectRequestCreate(
			v.ctx, v.reconciler.Client, v.reconciler.APIReader, v.log,
			s3StoreAccessor.url,
			s3StoreAccessor.bucketName,
			s3StoreAccessor.regionName,
			pathName,
			s3StoreAccessor.veleroNamespaceSecretKeyRef,
			vrg.Namespace,
			recoverGroup.Spec,
			veleroNamespaceName, kubeObjectsCaptureName(namePrefix, backupName, s3StoreAccessor.profileName),
			labels, annotations)
	} else {
		request, err = v.reconciler.kubeObjects.RecoverRequestCreate(
			v.ctx, v.reconciler.Client, v.reconciler.APIReader, v.log,
			s3StoreAccessor.url,
			s3StoreAccessor.bucketName,
			s3StoreAccessor.regionName,
			capturePathName,
			s3StoreAccessor.veleroNamespaceSecretKeyRef,
			sourceVrgNamespaceName, vrg.Namespace, recoverGroup, veleroNamespaceName,
			kubeObjectsCaptureName(captureNamePrefix, recoverGroup.BackupName, s3StoreAccessor.profileName),
			kubeObjectsRecoverName(recoverNamePrefix, groupNumber), labels, annotations)
	}

	return request, err
}

func (v *VRGInstance) kubeObjectsRecoveryStartOrResume(
	result *ctrl.Result, s3StoreAccessor s3StoreAccessor,
	sourceVrgNamespaceName, sourceVrgName string,
	capture *ramen.KubeObjectsCaptureIdentifier,
) error {
	vrg := v.instance
	veleroNamespaceName := v.veleroNamespaceName()
	labels := util.OwnerLabels(vrg.Namespace, vrg.Name)
	annotations := map[string]string{}
	groups := v.getRecoverGroups()
	requests := make([]kubeobjects.Request, len(groups)) // Request: interface for ProtectRequest, RecoverRequest
	log := v.log.WithValues("number", capture.Number, "profile", s3StoreAccessor.profileName)

	for groupNumber, recoverGroup := range groups {
		log1 := log.WithValues("group", groupNumber, "name", recoverGroup.BackupName)

		var request kubeobjects.Request

		var err error

		request, err = v.createRecoverOrProtectRequest(s3StoreAccessor, sourceVrgNamespaceName, sourceVrgName,
			capture, groupNumber, recoverGroup, veleroNamespaceName, labels, annotations)
		requests[groupNumber] = request

		if err == nil {
			log1.Info("Kube objects group recovered", "start", request.StartTime(), "end", request.EndTime())

			continue
		}

		if errors.Is(err, kubeobjects.RequestProcessingError{}) {
			log1.Info("Kube objects group recovering", "state", err.Error())

			return err
		}

		v.log.Error(err, "Kube objects group recover error")

		result.Requeue = true

		return err
	}

	startTime := requests[0].StartTime()
	duration := time.Since(startTime.Time)
	log.Info("Kube objects recovered", "groups", len(groups), "start", startTime, "duration", duration)

	return v.kubeObjectsRecoverRequestsDelete(result, veleroNamespaceName, labels)
}

func (v *VRGInstance) kubeObjectsRecoverRequestsDelete(
	result *ctrl.Result, veleroNamespaceName string, labels map[string]string,
) error {
	if err := v.reconciler.kubeObjects.RecoverRequestsDelete(
		v.ctx, v.reconciler.Client, veleroNamespaceName, labels,
	); err != nil {
		v.log.Error(err, "Kube objects recover requests delete error")

		result.Requeue = true

		return err
	}

	v.log.Info("Kube objects recover requests deleted")

	return nil
}

func (v *VRGInstance) veleroNamespaceName() string {
	if v.ramenConfig.KubeObjectProtection.VeleroNamespaceName != "" {
		return v.ramenConfig.KubeObjectProtection.VeleroNamespaceName
	}

	return VeleroNamespaceNameDefault
}

func (v *VRGInstance) kubeObjectProtectionDisabled(caller string) bool {
	vrgDisabled := v.instance.Spec.KubeObjectProtection == nil
	cmDisabled := v.ramenConfig.KubeObjectProtection.Disabled
	disabled := vrgDisabled || cmDisabled

	v.log.Info("Kube object protection", "disabled", disabled, "VRG", vrgDisabled, "configMap", cmDisabled, "for", caller)

	return disabled
}

func (v *VRGInstance) kubeObjectsProtectionDelete(result *ctrl.Result) error {
	if v.kubeObjectProtectionDisabled("deletion") {
		return nil
	}

	vrg := v.instance

	return v.kubeObjectsRecoverRequestsDelete(
		result,
		v.veleroNamespaceName(),
		util.OwnerLabels(vrg.Namespace, vrg.Name),
	)
}

func kubeObjectsRequestsWatch(b *builder.Builder, kubeObjects kubeobjects.RequestsManager) *builder.Builder {
	watch := func(request kubeobjects.Request) {
		src := &source.Kind{Type: request.Object()}
		b.Watches(
			src,
			handler.EnqueueRequestsFromMapFunc(func(o client.Object) []reconcile.Request {
				labels := o.GetLabels()
				log := func(s string) {
					ctrl.Log.WithName("VolumeReplicationGroup").Info(
						"Kube objects request updated; "+s,
						"kind", o.GetObjectKind(),
						"name", o.GetNamespace()+"/"+o.GetName(),
						"created", o.GetCreationTimestamp(),
						"gen", o.GetGeneration(),
						"ver", o.GetResourceVersion(),
						"labels", labels,
					)
				}

				if ownerNamespaceName, ownerName, ok := util.OwnerNamespaceNameAndName(labels); ok {
					log("owner labels found, enqueue VRG reconcile")

					return []reconcile.Request{
						{NamespacedName: types.NamespacedName{Namespace: ownerNamespaceName, Name: ownerName}},
					}
				}

				log("owner labels not found")

				return []reconcile.Request{}
			}),
			builder.WithPredicates(ResourceVersionUpdatePredicate{}),
		)
	}

	watch(kubeObjects.ProtectRequestNew())
	watch(kubeObjects.RecoverRequestNew())

	return b
}

func GetRecipeWithName(ctx context.Context, client client.Client, name, namespace string) (Recipe.Recipe, error) {
	recipe := &Recipe.Recipe{}

	err := client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, recipe)

	return *recipe, err
}

// input: recipe name, recipe name
// output: slice of capture objects that constitute the group
func (v *VRGInstance) getWorkflowAndRecipeFromName(
	recipeName, workflowName string) (*Recipe.Workflow, *Recipe.Recipe, error,
) {
	recipe, err := GetRecipeWithName(v.ctx, v.reconciler.Client, recipeName, v.instance.Namespace)
	if err != nil {
		return nil, nil, err
	}

	// get workflow
	workflow, err := getWorkflowByName(recipe, workflowName)
	if err != nil {
		return nil, nil, err
	}

	return workflow, &recipe, nil
}

func getWorkflowByName(recipe Recipe.Recipe, name string) (*Recipe.Workflow, error) {
	for _, workflow := range recipe.Spec.Workflows {
		if workflow.Name == name {
			return workflow, nil
		}
	}

	return nil, k8serrors.NewNotFound(schema.GroupResource{Resource: "Recipe.Spec.Workflow.Name"}, name)
}

func (v *VRGInstance) getCaptureGroupsFromWorkflow(
	recipe Recipe.Recipe, workflow Recipe.Workflow) ([]kubeobjects.CaptureSpec, error,
) {
	resources := make([]kubeobjects.CaptureSpec, len(workflow.Sequence))

	for index, resource := range workflow.Sequence {
		for resourceType := range resource {
			resourceName := resource[resourceType]

			captureInstance, err := getResourceAndConvertToCaptureGroup(recipe, resourceType, resourceName)
			if err != nil {
				return resources, err
			}

			resources[index] = *captureInstance
		}
	}

	return resources, nil
}

func (v *VRGInstance) getRestoreGroupsFromWorkflow(
	recipe Recipe.Recipe, workflow Recipe.Workflow) ([]kubeobjects.RecoverSpec, error,
) {
	resources := make([]kubeobjects.RecoverSpec, len(workflow.Sequence))

	for index, resource := range workflow.Sequence {
		// group: map[string]string, e.g. "group": "groupName", or "hook": "hookName"
		for resourceType := range resource {
			resourceName := resource[resourceType]

			captureInstance, err := getResourceAndConvertToRecoverGroup(recipe, resourceType, resourceName)
			if err != nil {
				return resources, err
			}

			resources[index] = *captureInstance
		}
	}

	return resources, nil
}

func getResourceAndConvertToCaptureGroup(
	recipe Recipe.Recipe, resourceType, name string) (*kubeobjects.CaptureSpec, error,
) {
	// check hooks OR groups
	if resourceType == "group" {
		for _, group := range recipe.Spec.Groups {
			if group.Name == name {
				return convertRecipeGroupToCaptureSpec(*group)
			}
		}

		return nil, k8serrors.NewNotFound(schema.GroupResource{Resource: "Recipe.Spec.Group.Name"}, name)
	}

	if resourceType == "hook" {
		hook, op, err := getHookAndOpFromRecipe(&recipe, name)
		if err != nil {
			return nil, k8serrors.NewNotFound(schema.GroupResource{Resource: "Recipe.Spec"}, resourceType)
		}

		return convertRecipeHookToCaptureSpec(*hook, *op)
	}

	return nil, k8serrors.NewNotFound(schema.GroupResource{Resource: "Recipe.Spec"}, resourceType)
}

// resource: could be Group or Hook
func getResourceAndConvertToRecoverGroup(
	recipe Recipe.Recipe, resourceType, name string) (*kubeobjects.RecoverSpec, error,
) {
	if resourceType == "group" {
		for _, group := range recipe.Spec.Groups {
			if group.Name == name {
				return convertRecipeGroupToRecoverSpec(*group)
			}
		}

		return nil, k8serrors.NewNotFound(schema.GroupResource{Resource: "Recipe.Spec.Group.Name"}, name)
	}

	if resourceType == "hook" {
		hook, op, err := getHookAndOpFromRecipe(&recipe, name)
		if err != nil {
			return nil, k8serrors.NewNotFound(schema.GroupResource{Resource: "Recipe.Spec"}, resourceType)
		}

		return convertRecipeHookToRecoverSpec(*hook, *op)
	}

	return nil, k8serrors.NewNotFound(schema.GroupResource{Resource: "Recipe.Spec"}, resourceType)
}

func getHookAndOpFromRecipe(recipe *Recipe.Recipe, name string) (*Recipe.Hook, *Recipe.Operation, error) {
	// hook can be made up of optionalPrefix/hookName; workflow sequence uses full name
	var prefix string

	var suffix string

	const containsSingleDelim = 2

	parts := strings.Split(name, "/")
	switch len(parts) {
	case 1:
		prefix = ""
		suffix = name
	case containsSingleDelim:
		prefix = parts[0]
		suffix = parts[1]
	default:
		return nil, nil, k8serrors.NewNotFound(schema.GroupResource{Resource: "Recipe.Spec.Hook.Name"}, name)
	}

	// match prefix THEN suffix
	for _, hook := range recipe.Spec.Hooks {
		if hook.Name == prefix {
			for _, op := range hook.Ops {
				if op.Name == suffix {
					return hook, op, nil
				}
			}
		}
	}

	return nil, nil, k8serrors.NewNotFound(schema.GroupResource{Resource: "Recipe.Spec.Hook.Name"}, name)
}

// TODO: complete functionality - add Hook support to KubeResourcesSpec, then copy in Velero object creation
func convertRecipeHookToCaptureSpec(
	hook Recipe.Hook, op Recipe.Operation) (*kubeobjects.CaptureSpec, error,
) {
	hookName := hook.Name + "-" + op.Name

	hooks, err := getHookSpecFromHook(hook, op)
	if err != nil {
		return nil, err
	}

	captureSpec := kubeobjects.CaptureSpec{
		Name: hookName,
		Spec: kubeobjects.Spec{
			KubeResourcesSpec: kubeobjects.KubeResourcesSpec{
				IncludedResources: []string{"pod"},
				ExcludedResources: []string{},
				Hooks:             hooks,
			},
			LabelSelector:           &hooks[0].LabelSelector,
			IncludeClusterResources: new(bool),
		},
	}

	return &captureSpec, nil
}

func convertRecipeHookToRecoverSpec(hook Recipe.Hook, op Recipe.Operation) (*kubeobjects.RecoverSpec, error) {
	hooks, err := getHookSpecFromHook(hook, op)
	if err != nil {
		return nil, err
	}

	return &kubeobjects.RecoverSpec{
		// BackupName: arbitrary fixed string to designate that this is will be a Backup, not Restore, object
		BackupName: ramen.ReservedBackupName,
		Spec: kubeobjects.Spec{
			KubeResourcesSpec: kubeobjects.KubeResourcesSpec{
				IncludedResources: []string{"pod"},
				ExcludedResources: []string{},
				Hooks:             hooks,
			},
			LabelSelector:           &hooks[0].LabelSelector,
			IncludeClusterResources: new(bool),
		},
	}, nil
}

func getHookSpecFromHook(hook Recipe.Hook, op Recipe.Operation) ([]kubeobjects.HookSpec, error) {
	labelSelector, err := getLabelSelectorFromString(hook.LabelSelector)
	if err != nil {
		return nil, err
	}

	return []kubeobjects.HookSpec{
		{
			Name:          op.Name,
			Type:          hook.Type,
			Timeout:       metav1.Duration{Duration: time.Duration(op.Timeout * int(time.Second))},
			Container:     op.Container,
			Command:       op.Command,
			LabelSelector: *labelSelector,
		},
	}, nil
}

func convertRecipeGroupToRecoverSpec(group Recipe.Group) (*kubeobjects.RecoverSpec, error) {
	labelSelector, err := getLabelSelectorFromString(group.LabelSelector)
	if err != nil {
		return nil, err
	}

	return &kubeobjects.RecoverSpec{
		BackupName: group.BackupRef,
		Spec: kubeobjects.Spec{
			KubeResourcesSpec: kubeobjects.KubeResourcesSpec{
				IncludedResources: group.IncludedResourceTypes,
				ExcludedResources: group.ExcludedResourceTypes,
			},
			LabelSelector:           labelSelector,
			OrLabelSelectors:        []*metav1.LabelSelector{},
			IncludeClusterResources: group.IncludeClusterResources,
		},
	}, nil
}

func convertRecipeGroupToCaptureSpec(group Recipe.Group) (*kubeobjects.CaptureSpec, error) {
	labelSelector, err := getLabelSelectorFromString(group.LabelSelector)
	if err != nil {
		return nil, err
	}

	captureSpec := kubeobjects.CaptureSpec{
		Name: group.Name,
		// TODO: add backupRef/backupName here?
		Spec: kubeobjects.Spec{
			KubeResourcesSpec: kubeobjects.KubeResourcesSpec{
				IncludedResources: group.IncludedResourceTypes,
				ExcludedResources: group.ExcludedResourceTypes,
			},
			LabelSelector:           labelSelector,
			OrLabelSelectors:        []*metav1.LabelSelector{},
			IncludeClusterResources: group.IncludeClusterResources,
		},
	}

	return &captureSpec, nil
}

func getLabelSelectorFromString(labels string) (*metav1.LabelSelector, error) {
	// velero bug: https://github.com/vmware-tanzu/velero/issues/2083
	// adding labelSelector: {} will create validation error; omit to avoid this
	if labels == "" {
		return nil, nil
	}

	return metav1.ParseToLabelSelector(labels)
}
