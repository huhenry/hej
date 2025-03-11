package filter_test

import (
	. "github.com/onsi/ginkgo"

	. "github.com/huhenry/hej/pkg/handler/microapp/filter"
	. "github.com/onsi/gomega"
)

var _ = Describe("FilterParser", func() {

	Describe("test PraserPrams", func() {
		Context("test Praserprams", func() {
			It("应该是一个filterlist", func() {

			})
		})

		RegisterFailHandler(Fail)

	})

	Describe("test Prase", func() {
		Context("test TimePraser", func() {
			It("应该是一个filterlist", func() {

			})
		})

		Context("test NamePraser", func() {
			It("应该是一个filterlist", func() {
				name := "app"
				filterlist := make([]Filter, 0)
				namePraser := NamePraser{NameFilter{Name: name}}
				filterlist = namePraser.Prase(filterlist)

			})
		})

		Context("test CreatorPraser", func() {
			It("应该是一个filterlist", func() {
				creator := "lisi"
				filterlist := make([]Filter, 0)
				creatorPraser := CreatorPraser{CreatorFilter{Creator: creator}}
				filterlist = creatorPraser.Prase(filterlist)

			})
		})

		RegisterFailHandler(Fail)

	})

})
