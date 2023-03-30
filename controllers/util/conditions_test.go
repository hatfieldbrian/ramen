// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/ramendr/ramen/controllers/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ConditionTrueAndCurrentTest", func() {
	object := metav1.ObjectMeta{Generation: 3}
	const conditionType = "asdf"
	var conditions []metav1.Condition
	BeforeEach(func() {
		conditions = make([]metav1.Condition, 2)
	})
	expectTo := func(matcher types.GomegaMatcher) {
		Expect(util.ConditionTrueAndCurrentTest(object.GetGeneration(), conditions, conditionType, testLogger)).To(matcher)
	}
	When("condition is absent", func() {
		It("returns false", func() { expectTo(BeFalse()) })
	})
	When("condition is present", func() {
		var condition *metav1.Condition
		BeforeEach(func() {
			condition = &conditions[0]
			*condition = metav1.Condition{
				Type: conditionType,
			}
		})
		When("observed generation is less than current", func() {
			BeforeEach(func() { condition.ObservedGeneration = object.GetGeneration() - 1 })
			It("returns false", func() { expectTo(BeFalse()) })
		})
		When("observed generation is equal to current", func() {
			BeforeEach(func() { condition.ObservedGeneration = object.GetGeneration() })
			When("status is unknown", func() {
				BeforeEach(func() { condition.Status = metav1.ConditionUnknown })
				It("returns false", func() { expectTo(BeFalse()) })
			})
			When("status is true", func() {
				BeforeEach(func() { condition.Status = metav1.ConditionTrue })
				It("returns true", func() { expectTo(BeTrue()) })
			})
			When("status is false", func() {
				BeforeEach(func() { condition.Status = metav1.ConditionFalse })
				It("returns false", func() { expectTo(BeFalse()) })
			})
		})
	})
})
