package controllers

import (
	"context"
	"fmt"
	"net/http"

	ramen "github.com/ramendr/ramen/api/v1alpha1"
	recipe "github.com/ramendr/recipe/api/v1alpha1"
	authentication "k8s.io/api/authentication/v1"
	authorization "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

//nolint:lll
//+kubebuilder:webhook:path=/validate-ramendr-openshift-io-v1alpha1-volumereplicationgroup,mutating=false,failurePolicy=fail,sideEffects=None,groups=ramendr.openshift.io,resources=volumereplicationgroups,verbs=create;update,versions=v1alpha1,name=vvolumereplicationgroup.kb.io,admissionReviewVersions=v1

func vrgValidatorWebhookRegister(mgr manager.Manager) {
	mgr.GetWebhookServer().Register(
		"/validate-ramendr-openshift-io-v1alpha1-volumereplicationgroup",
		&admission.Webhook{Handler: &vrgValidator{
			client:  mgr.GetClient(),
			decoder: func() *admission.Decoder { d, _ := admission.NewDecoder(mgr.GetScheme()); return d }(), //nolint:errcheck,nlreturn,lll
			// TODO fix with controller-runtime v0.15+
			// decoder: admission.NewDecoder(mgr.GetScheme()),
		}},
	)
}

type vrgValidator struct {
	client  client.Client
	decoder *admission.Decoder
}

var _ admission.Handler = &vrgValidator{}

func (v *vrgValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	vrg := &ramen.VolumeReplicationGroup{}
	if err := v.decoder.Decode(req, vrg); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	if vrg.Spec.KubeObjectProtection == nil ||
		vrg.Spec.KubeObjectProtection.RecipeRef == nil {
		return admission.Allowed("recipe not specified")
	}

	recipeNamespacedName := types.NamespacedName{
		Namespace: vrg.Namespace,
		Name:      vrg.Spec.KubeObjectProtection.RecipeRef.Name,
	}

	recipe := &recipe.Recipe{}
	if err := v.client.Get(ctx, recipeNamespacedName, recipe); err != nil {
		return admission.Errored(http.StatusInternalServerError,
			fmt.Errorf("recipe %#v retrieval error: %w", recipeNamespacedName, err))
	}

	namespaceNames := make(sets.Set[string], 0)

	if recipe.Spec.Volumes != nil {
		namespaceNames.Insert(recipe.Spec.Volumes.IncludedNamespaces...)
	}

	for _, group := range recipe.Spec.Groups {
		namespaceNames.Insert(group.IncludedNamespaces...)
	}

	for _, namespaceName := range namespaceNames.UnsortedList() {
		accessReview := vrgCreateAccessReview(req.UserInfo, namespaceName)
		if err := v.client.Create(ctx, accessReview); err != nil {
			return admission.Errored(http.StatusInternalServerError,
				fmt.Errorf("namespace %v access review creation error: %w",
					namespaceName, err))
		}

		if !accessReview.Status.Allowed {
			return admission.Denied(
				fmt.Sprintf("%v %v creation access denied: %+v",
					vrgKindName,
					accessReview.Spec.ResourceAttributes.Namespace,
					accessReview.Status,
				),
			)
		}
	}

	return admission.Allowed("")
}

func vrgCreateAccessReview(userInfo authentication.UserInfo, namespaceName string,
) *authorization.SubjectAccessReview {
	extra := make(map[string]authorization.ExtraValue, len(userInfo.Extra))
	for key, value := range userInfo.Extra {
		extra[key] = authorization.ExtraValue(value)
	}

	return &authorization.SubjectAccessReview{
		Spec: authorization.SubjectAccessReviewSpec{
			ResourceAttributes: &authorization.ResourceAttributes{
				Namespace: namespaceName,
				Verb:      "create",
				Group:     ramen.GroupVersion.Group,
				Version:   ramen.GroupVersion.Version,
				Resource:  vrgResourceName,
			},
			User:   userInfo.Username,
			Groups: userInfo.Groups,
			Extra:  extra,
			UID:    userInfo.UID,
		},
	}
}
