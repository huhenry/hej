package traffic

import (
	"fmt"

	"github.com/huhenry/hej/pkg/common/app"
	customErrors "github.com/huhenry/hej/pkg/errors"
	"github.com/huhenry/hej/pkg/handler"
	"github.com/huhenry/hej/pkg/handler/audit"
	micro "github.com/huhenry/hej/pkg/microapp"
	"github.com/huhenry/hej/pkg/traffic"
	"github.com/huhenry/hej/pkg/traffic/policy"
	"github.com/kataras/iris/v12"
)

func GetPolicy(ctx iris.Context) {
	name := ctx.Params().GetString("name")
	appCtx := handler.ExtractAppContext(ctx)
	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}
	settings, err := traffic.Policy().GetSettings(resource, name)

	if err != nil {
		handler.ResponseErr(ctx, err)
	} else {
		handler.ResponseOk(ctx, settings)
	}
}

func SetPolicy(ctx iris.Context) {
	application := ctx.Params().GetString("application")
	name := ctx.Params().GetString("name")
	settings := &policy.Settings{}
	err := ctx.ReadJSON(settings)
	if err != nil {
		logger.Errorf("set policy failed: %v", err)
		handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, "数据格式错误")
		return
	}

	appCtx := handler.ExtractAppContext(ctx)
	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}

	if settings.Authorization != nil && len(settings.Authorization.Services) > 0 {
		microservices, err := micro.MicroService().List(resource, application, false)
		if err != nil {
			handler.ResponseErr(ctx, err)
			return
		}
		set := make(map[string]struct{})
		for i := range microservices {
			set[microservices[i].ServiceName] = struct{}{}
		}
		for _, svc := range settings.Authorization.Services {
			if _, ok := set[svc]; !ok {
				handler.ResponseErr(ctx, customErrors.BadRequest(fmt.Sprintf("访问鉴权配置的服务%s不存在", svc)))
				return
			}
		}
	}

	err = traffic.Policy().SetSettings(resource, application, name, settings)
	if err != nil {
		handler.ResponseErr(ctx, err)
	} else {
		handler.SendAudit(audit.ModuleMicroService, audit.ActionTrafficPolicy, application+"/"+name, ctx)
		handler.ResponseOk(ctx, nil)
	}
}
