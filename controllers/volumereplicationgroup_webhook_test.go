// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package controllers_test

import (
	//	"context"
	//	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	ramen "github.com/ramendr/ramen/api/v1alpha1"
	//	"github.com/ramendr/ramen/controllers"
	recipe "github.com/ramendr/recipe/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	// "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/runtime/schema"
	// "k8s.io/apimachinery/pkg/types"
)

var _ = Describe("VolumeReplicationGroupWebhook", func() {
	var (
		ns  *corev1.Namespace
		r   *recipe.Recipe
		vrg *ramen.VolumeReplicationGroup
		err error
	)

	nsCreate := func() *corev1.Namespace {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{GenerateName: vrgTestNamespaceBase},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		return ns
	}
	nsDelete := func(ns *corev1.Namespace) {
		Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
	}
	group := func(typeName string, namespaceNames ...string) *recipe.Group {
		return &recipe.Group{
			Type:               typeName,
			IncludedNamespaces: namespaceNames,
		}
	}
	volumes := func(namespaceNames ...string) *recipe.Group {
		return group("volume", namespaceNames...)
	}
	resources := func(namespaceNames ...string) *recipe.Group {
		return group("resource", namespaceNames...)
	}
	recipeCreate := func(volumes *recipe.Group,
		groups ...*recipe.Group,
	) {
		r = &recipe.Recipe{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      "r",
			},
			Spec: recipe.RecipeSpec{
				Volumes: volumes,
				Groups:  groups,
			},
		}
		Expect(k8sClient.Create(ctx, r)).To(Succeed())
	}
	recipeDelete := func() {
		Expect(k8sClient.Delete(ctx, r)).To(Succeed())
	}
	localRef := func(name string) *corev1.LocalObjectReference {
		return &corev1.LocalObjectReference{Name: name}
	}
	vrgDefine := func(recipeRef *corev1.LocalObjectReference) {
		vrg = &ramen.VolumeReplicationGroup{
			ObjectMeta: metav1.ObjectMeta{Namespace: ns.Name, Name: "a"},
			Spec: ramen.VolumeReplicationGroupSpec{
				S3Profiles:       []string{""},
				ReplicationState: ramen.Primary,
				KubeObjectProtection: &ramen.KubeObjectProtectionSpec{
					RecipeRef: recipeRef,
				},
			},
		}
	}
	vrgCreate := func() {
		err = k8sClient.Create(ctx, vrg)
	}
	vrgDelete := func() {
		Expect(k8sClient.Delete(ctx, vrg)).To(Succeed())
	}

	BeforeEach(func() {
		ns = nsCreate()
	})
	AfterEach(func() {
		nsDelete(ns)
	})
	JustBeforeEach(func() {
		vrgCreate()
	})
	When("a VRG creation request is submitted without a recipe reference", func() {
		BeforeEach(func() {
			vrgDefine(nil)
		})
		AfterEach(func() {
			vrgDelete()
		})
		It("should allow it", func() { Expect(err).ToNot(HaveOccurred()) })
	})
	When("a VRG creation request is submitted referencing an absent recipe", func() {
		BeforeEach(func() {
			vrgDefine(localRef("asdf"))
		})
		It("should deny it", func() { Expect(err).To(HaveOccurred()) })
	})
	When("a VRG creation request is submitted referencing a recipe without groups", func() {
		BeforeEach(func() {
			recipeCreate(nil)
			vrgDefine(localRef(r.Name))
		})
		AfterEach(func() {
			recipeDelete()
		})
		It("should allow it", func() { Expect(err).ToNot(HaveOccurred()) })
	})
	When("a VRG creation request is submitted referencing a recipe that references other namespaces"+
		"that the requestor has permission to create VRGs in", func() {
		var ns1 *corev1.Namespace
		BeforeEach(func() {
			ns1 = nsCreate()
			recipeCreate(volumes(ns1.Name), resources(ns1.Name))
			vrgDefine(localRef(r.Name))
		})
		AfterEach(func() {
			recipeDelete()
			nsDelete(ns1)
		})
		It("should allow it", func() { Expect(err).ToNot(HaveOccurred()) })
	})
	When("a VRG creation request is submitted referencing a recipe that references other namespaces"+
		"that the requestor does not have permission to create VRGs in", func() {
		var ns1 *corev1.Namespace
		BeforeEach(func() {
			// TODO envTest.AddUser
			// TODO give user permission to update VRG in ns, but not ns1
			// TODO impersonate user for client.Create(vrg)
			ns1 = nsCreate()
			recipeCreate(volumes(ns1.Name), resources(ns1.Name))
			vrgDefine(localRef(r.Name))
		})
		AfterEach(func() {
			recipeDelete()
			nsDelete(ns1)
		})
		It("should deny it", func() { Expect(err).ToNot(HaveOccurred()) })
	})
})
