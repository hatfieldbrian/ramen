// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var volumereplicationgrouplog = logf.Log.WithName("volumereplicationgroup-resource")

func (r *VolumeReplicationGroup) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-ramendr-openshift-io-v1alpha1-volumereplicationgroup,mutating=false,failurePolicy=fail,sideEffects=None,groups=ramendr.openshift.io,resources=volumereplicationgroups,verbs=create;update,versions=v1alpha1,name=vvolumereplicationgroup.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &VolumeReplicationGroup{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *VolumeReplicationGroup) ValidateCreate() error {
	volumereplicationgrouplog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *VolumeReplicationGroup) ValidateUpdate(old runtime.Object) error {
	volumereplicationgrouplog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *VolumeReplicationGroup) ValidateDelete() error {
	volumereplicationgrouplog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
