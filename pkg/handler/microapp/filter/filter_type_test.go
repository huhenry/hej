package filter_test

import (
	. "github.com/huhenry/hej/pkg/handler/microapp/filter"
	v1beta1 "github.com/huhenry/hej/pkg/microapp/v1beta1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("FilterType", func() {

	Context("测试NameFilter", func() {
		It("应该返回具有指定name的microapps", func() {

			NameFilter := NameFilter{Name: "app"}
			microAppAll := make([]v1beta1.MicroApp, 0)
			microapps := NameFilter.Filte(microAppAll)
			for _, microappItem := range microapps {
				Expect(microappItem.Name).To(Equal("app"))
			}
		})
		RegisterFailHandler(Fail)

	})

	Context("测试CreatorFilter", func() {
		It("应该返回具有指定creator的microapps", func() {

			CreatorFilter := CreatorFilter{Creator: "lisi"}
			microAppAll := make([]v1beta1.MicroApp, 0)
			microapps := CreatorFilter.Filte(microAppAll)
			for _, microappItem := range microapps {
				Expect(microappItem.Creator).To(Equal("lisi"))
			}
		})
		RegisterFailHandler(Fail)

	})

})
