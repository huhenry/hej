package handler

import (
	"errors"
	"net/http"
	"strings"

	backendV1 "github.com/huhenry/hej/pkg/backend/v1"
	"github.com/huhenry/hej/pkg/define"
	customerrors "github.com/huhenry/hej/pkg/errors"
	"github.com/huhenry/hej/pkg/handler/audit"
	"github.com/kataras/iris/v12"
)

var MsgTrans = map[int]string{
	customerrors.CodeBadParametersErr: "参数错误",
	customerrors.CodeDynamicClientErr: "集群连接失败",
}

func Response(ctx iris.Context, code int, msg string) {
	resp := NewRespJson()
	resp.Status = code
	resp.Msg = msg
	resp.Data = nil
	resp.Detail = ""
	ctx.JSON(resp)
	responseLog(ctx, msg)
	return
}

func ResponseMessageList(ctx iris.Context, code int, msg []string) {
	resp := &RespMsgListJson{
		Status: code,
		Msg:    msg,
		Data:   nil,
		Detail: "",
	}
	ctx.JSON(resp)
	responseLog(ctx, msg)
	return
}

func ResponseErr(ctx iris.Context, err error) {
	for i := range errorHandlerChain {
		if resp, ok := errorHandlerChain[i](ctx, err); ok {
			ctx.JSON(resp)
			responseLog(ctx, err.Error())
			return
		}
	}

	resp := &RespJson{
		Status: http.StatusInternalServerError,
		Msg:    err.Error(),
	}
	ctx.JSON(resp)
	responseLog(ctx, err.Error())
}

func RespondWithError(ctx iris.Context, code int, err error) {
	resp := &RespJson{
		Status: code,
		Msg:    err.Error(),
	}
	ctx.JSON(resp)
	responseLog(ctx, err.Error())
}

func RespondWithDetailedError(ctx iris.Context, err error) {
	var httpErr *customerrors.HttpRespError
	resp := &RespJson{}
	if errors.As(err, &httpErr) {
		resp.FromHttpError(httpErr)

	} else {
		resp = &RespJson{
			Status: http.StatusInternalServerError,
			Msg:    "服务内部错误",
			Detail: err.Error(),
		}
	}
	ctx.JSON(resp)
	responseLog(ctx, err.Error())
}

func (resp *RespJson) FromHttpError(httpErr *customerrors.HttpRespError) {
	resp.Status = httpErr.HTTPStatus
	resp.Msg = httpErr.Message

	if msg, ok := MsgTrans[httpErr.Code]; ok {
		resp.Msg = msg
	}

	if errors.Unwrap(httpErr) != nil {
		resp.Detail = errors.Unwrap(httpErr).Error()
	}

}

func ResponseOk(ctx iris.Context, respdata interface{}) {
	resp := NewRespJson()
	resp.Status = define.ST_OK
	resp.Msg = ResponseStatusOk
	resp.Data = respdata

	_, err := ctx.JSON(resp)
	if err != nil {
		logger.Errorf("ResponseOk ctx.JSON err:%s", err)
	}
	ctx.Values().Set(define.CtxRespStsKey, ResponseStatusOk)
	responseLog(ctx, respdata)
	return
}

func SendAudit(module, action, target string, ctx iris.Context) {
	entry := backendV1.Audit{}
	entry.Module = module
	entry.Action = action
	entry.Target = target

	list := []*backendV1.Audit{&entry}
	err := auditFill(list, ctx)
	if err != nil {
		logger.Errorf("try to generate entry message failed: %v", err)
	} else {
		audit.Send(list)
	}
}

func auditFill(audits []*backendV1.Audit, ctx iris.Context) error {
	var userContext *UserContext
	var appContext *AppClusterContext
	ok := true

	userInterface := ctx.Values().Get(define.UserContextKey)
	if userInterface == nil {
		return errors.New("no user context found")
	} else if userContext, ok = userInterface.(*UserContext); !ok {
		return errors.New("no user context found")
	}

	appInterface := ctx.Values().Get(define.AppContextKey)
	if appInterface != nil {
		if appContext, ok = appInterface.(*AppClusterContext); !ok {
			return errors.New("no app context found")
		}
	}

	for i := range audits {
		audit := audits[i]
		audit.User = userContext.Name
		audit.UserIp = getIp(ctx)
		if appInterface != nil {
			audit.AppId = appContext.AppId
			audit.Cluster = appContext.ClusterName
			audit.NamespaceId = int64(appContext.NamespaceId)
		} else {
			cluster := ctx.Params().GetString(define.ClusterKey)
			audit.Cluster = cluster
		}

	}

	return nil
}

func getIp(ctx iris.Context) string {
	ips := ctx.Request().Header.Get("X-Forwarded-For")
	if ips != "" {
		if ipList := strings.Split(ips, ","); len(ipList) > 0 {
			return ipList[0]
		}
	}
	addr := ctx.Request().RemoteAddr
	str := strings.Split(addr, ":")
	if len(str) > 1 {
		return str[0]
	} else {
		return addr
	}
}
