package audit

import (
	"github.com/huhenry/hej/pkg/backend"
	v1 "github.com/huhenry/hej/pkg/backend/v1"
	"github.com/huhenry/hej/pkg/log"
)

var logger = log.RegisterScope("audit-middleware")

const (
	ModuleMicroApplication = "微服务应用"
	ModuleMicroService     = "微服务"
	ModuleGateway          = "微服务网关"
	ModuleCanary           = "灰度发布"
	ModuleIstio            = "Istio"
	ModuleServiceEntry     = "外部服务"
	ActionUnbinding        = "移除"
	ActionTrafficPolicy    = "流量治理"
	ActionInstall          = "安装"
	ActionUnInstall        = "卸载"
	ActionCreate           = "创建"
	ActionUpdate           = "修改"
	ActionDelete           = "删除"
	ActionGOOFFLINE        = "下线版本"
	ActionPut              = "编辑"
)

func Send(audits []*v1.Audit) {
	for i := range audits {
		audit := audits[i]
		executor := backend.GetClient().V1().AdminExecutor()
		if err := backend.GetClient().V1().Audit().Create(executor, audit); err != nil {
			logger.Errorf("invoke audit interface failed due to: %v", err)
		}
	}
}
