package handler

import (
	"strings"

	"github.com/huhenry/hej/pkg/errors"

	"github.com/huhenry/hej/pkg/backend"
	"github.com/huhenry/hej/pkg/define"
	"github.com/huhenry/hej/pkg/log"
	"github.com/kataras/iris/v12"
)

var logger = log.RegisterScope("middleware")

type AppClusterContext struct {
	AppId         int64
	AppName       string
	ClusterId     int64
	ClusterName   string
	Namespace     string
	NamespaceId   int
	KubeNamespace string
}

type UserContext struct {
	Token   string
	Name    string
	IsAdmin bool
}

func ExtractAppCluster(ctx iris.Context) {
	app, err := ctx.Params().GetInt64("app")
	if err != nil || app <= 0 {
		Response(ctx, 400, "app参数非法")
		return
	}
	cluster := ctx.Params().Get("cluster")
	if cluster == "" {
		Response(ctx, 400, "cluster参数非法")
		return
	}
	info, err := backend.GetClient().V1().App().AppInfoCache(app)
	if err != nil {
		logger.Errorf("无法从缓存中获取部门信息:%v", err)
		Response(ctx, 400, "部门信息获取失败")
		return
	}
	ctx.Values().Set(define.AppContextKey, &AppClusterContext{
		AppId:         app,
		AppName:       info.Name,
		ClusterName:   cluster,
		NamespaceId:   info.Namespace.Id,
		Namespace:     info.Namespace.Name,
		KubeNamespace: info.Namespace.KubeNamespace,
	})
	ctx.Next()
}

func VerifyToken(ctx iris.Context) {
	token := ctx.Request().Header.Get("Authorization")
	if token == "" {
		Response(ctx, 401, "token非法")
		return
	}
	tokens := strings.Split(token, " ")
	if len(tokens) != 2 {
		Response(ctx, 401, "token非法")
		return
	}
	user, err := backend.GetClient().V1().User().VerifyUserByToken(tokens[1])
	if err != nil {
		RespondWithDetailedError(ctx, errors.OriginErr(err))
		return
	}
	if user == nil {
		logger.Warnf("got nil user from backend via VerifyUserByToken")
		Response(ctx, 401, "token非法")
		return
	}

	ctx.Values().Set(define.UserContextKey, &UserContext{
		Name:    user.Name,
		Token:   tokens[1],
		IsAdmin: user.Admin,
	})
	ctx.Next()
}

func MustAdmin(ctx iris.Context) {
	userContext := ExtractUserContext(ctx)
	isAdmin := backend.GetClient().V1().User().IsAdmin(userContext.Token)
	if !isAdmin {
		Response(ctx, 403, "未授权")
		return
	}
	ctx.Next()
}
