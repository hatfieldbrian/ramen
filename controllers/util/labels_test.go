// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/ramendr/ramen/controllers/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("labels", func() {
	var object metav1.ObjectMeta

	BeforeEach(func() {
		object = metav1.ObjectMeta{}
	})

	Describe("AddLabels", func() {
		var toAdd, existing, merged util.Labels
		var grew bool

		BeforeEach(func() {
		})
		JustBeforeEach(func() {
			merged, grew = util.AddLabels(toAdd, existing)
		})
		Context("existing is nil", func() {
			BeforeEach(func() {
				toAdd = util.Labels{"k": "v"}
				existing = object.GetLabels()
				Expect(existing).To(BeNil())
			})
			It("adds the labels", func() {
				Expect(merged).To(Equal(toAdd))
				Expect(grew).To(Equal(true))
			})
		})
	})
})
