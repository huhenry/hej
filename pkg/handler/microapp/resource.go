package microapp

import (
	customerrors "github.com/huhenry/hej/pkg/errors"
	"github.com/huhenry/hej/pkg/handler"
	"github.com/huhenry/hej/pkg/microapp"
	"github.com/kataras/iris/v12"
)

func GetAvailableServices(ctx iris.Context) {
	appCtx := handler.ExtractAppContext(ctx)
	userCtx := handler.ExtractUserContext(ctx)
	list, err := microapp.Resource().
		GetBindingAvailable(appCtx.ClusterName, appCtx.AppId, userCtx.Token)

	if err != nil {
		handler.RespondWithDetailedError(ctx, customerrors.CustomClientErr("获取服务列表失败", err))
	} else {
		handler.ResponseOk(ctx, list)
	}
}
