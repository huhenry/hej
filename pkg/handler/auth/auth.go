package auth

import (
	"github.com/huhenry/hej/pkg/backend"
	v1 "github.com/huhenry/hej/pkg/backend/v1"
	"github.com/huhenry/hej/pkg/define"
	customErrors "github.com/huhenry/hej/pkg/errors"
	"github.com/huhenry/hej/pkg/handler"
	"github.com/huhenry/hej/pkg/log"
	"github.com/kataras/iris/v12"
)

type Permission struct {
	Resource string
	Action   string
}

var logger = log.RegisterScope("auth-middleware")

var MR = Permission{Resource: define.AuthMicroAppResource, Action: define.AuthActionRead}
var MC = Permission{Resource: define.AuthMicroAppResource, Action: define.AuthActionCreate}
var MU = Permission{Resource: define.AuthMicroAppResource, Action: define.AuthActionUpdate}
var MD = Permission{Resource: define.AuthMicroAppResource, Action: define.AuthActionDelete}
var SR = Permission{Resource: define.AuthServiceResource, Action: define.AuthActionRead}
var SC = Permission{Resource: define.AuthServiceResource, Action: define.AuthActionCreate}
var SU = Permission{Resource: define.AuthServiceResource, Action: define.AuthActionUpdate}
var SD = Permission{Resource: define.AuthServiceResource, Action: define.AuthActionDelete}
var DR = Permission{Resource: define.AuthDeploymentResource, Action: define.AuthActionRead}
var DC = Permission{Resource: define.AuthDeploymentResource, Action: define.AuthActionCreate}
var DU = Permission{Resource: define.AuthDeploymentResource, Action: define.AuthActionUpdate}
var DD = Permission{Resource: define.AuthDeploymentResource, Action: define.AuthActionDelete}
var CR = Permission{Resource: define.AuthCanaryResource, Action: define.AuthActionRead}
var CC = Permission{Resource: define.AuthCanaryResource, Action: define.AuthActionCreate}
var CU = Permission{Resource: define.AuthCanaryResource, Action: define.AuthActionUpdate}
var CD = Permission{Resource: define.AuthCanaryResource, Action: define.AuthActionDelete}
var CO = Permission{Resource: define.AuthCanaryResource, Action: define.AuthActionGoOffline}

func CheckPermissions(namespaceId int, appId int64, usrCtx *handler.UserContext, permissions ...Permission) bool {
	// if user is admin, just skip permission check
	if usrCtx.IsAdmin {
		return true
	}

	perMap, err := backend.GetClient().V1().Auth().
		GetPermission(namespaceId, appId, usrCtx.Token)
	if err != nil {
		logger.Errorf("occurs error when getting user permissions :%v", err)
		return false
	}

	var contains = func(list []v1.Permission, action string) bool {
		for _, item := range list {
			if item.Name == action && item.Enable == v1.ICanAccess {
				return true
			}
		}

		return false
	}

	for _, permission := range permissions {
		list := perMap[permission.Resource]
		logger.Debugf("permissions resource :%s action :%+v", permission.Resource, list)
		if !contains(list, permission.Action) {
			logger.Infof("permission for %s - %s not satisfied", permission.Resource, permission.Action)
			return false
		}
	}

	return true
}

func HasAuthorization(ctx iris.Context, permissions ...Permission) bool {
	userContext := handler.ExtractUserContext(ctx)
	appContext := handler.ExtractAppContext(ctx)
	if userContext == nil || appContext == nil {
		logger.Warnf("fail to extract user or app context for auth")
		handler.Response(ctx, customErrors.StatusCodeUnAuthorized, "未授权")
		return false
	}

	return CheckPermissions(appContext.NamespaceId, appContext.AppId, userContext, permissions...)
}

func Handler(permissions ...Permission) func(iris.Context) {
	return func(ctx iris.Context) {
		result := HasAuthorization(ctx, permissions...)
		if !result {
			handler.Response(ctx, customErrors.StatusCodeUnAuthorized, "未授权")
			return
		}

		ctx.Next()
	}
}

func HandlerWithMsg(message string, permissions ...Permission) func(iris.Context) {
	if len(message) == 0 {
		message = "未授权"
	}
	return func(ctx iris.Context) {
		result := HasAuthorization(ctx, permissions...)
		if !result {
			handler.Response(ctx, customErrors.StatusCodeUnAuthorized, message)
			return
		}

		ctx.Next()
	}
}
