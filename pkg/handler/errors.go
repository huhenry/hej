package handler

import (
	"fmt"
	"net/http"

	errors2 "github.com/huhenry/hej/pkg/errors"
	"github.com/kataras/iris/v12"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

type ErrorHandler func(ctx iris.Context, err error) (*RespJson, bool)

var errorHandlerChain = []ErrorHandler{
	upstreamErrorHandler,
	k8sNotFoundHandler,
	k8sAlreadyExistsHandler,
	customBadRequestHandler,
	customConflictHandler,
	defaultErrorHandler,
}

func upstreamErrorHandler(ctx iris.Context, err error) (*RespJson, bool) {
	ue, ok := err.(*errors2.UpstreamError)
	if ok {
		resp := NewRespJson()
		resp.Status = ue.Code
		resp.Msg = ue.Msg

		return resp, true
	}

	return nil, false
}

func k8sNotFoundHandler(ctx iris.Context, err error) (*RespJson, bool) {
	if k8serrors.IsNotFound(err) {
		if statusError, ok := err.(*k8serrors.StatusError); ok {
			name := statusError.ErrStatus.Details.Name
			cluster := TryGetCluster(ctx)
			message := ""
			if cluster != "" {
				message = fmt.Sprintf("%s在集群[%s]不存在", name, cluster)
			} else {
				message = fmt.Sprintf("%s在集群中不存在", name)
			}

			return &RespJson{
				Status: http.StatusInternalServerError,
				Msg:    message,
			}, true
		}
	}

	return nil, false
}

func k8sAlreadyExistsHandler(ctx iris.Context, err error) (*RespJson, bool) {
	if k8serrors.IsAlreadyExists(err) {
		if statusError, ok := err.(*k8serrors.StatusError); ok {
			name := statusError.ErrStatus.Details.Name
			cluster := TryGetCluster(ctx)
			message := ""
			if cluster != "" {
				message = fmt.Sprintf("%s在集群[%s]中已存在", name, cluster)
			} else {
				message = fmt.Sprintf("%s在集群中已存在", name)
			}

			return &RespJson{
				Status: http.StatusConflict,
				Msg:    message,
			}, true
		}
	}

	return nil, false
}

func customError2Response(err errors2.CustomErrorAdapter) *RespJson {
	return &RespJson{
		Status: err.Code,
		Msg:    err.Message,
	}
}

func customBadRequestHandler(ctx iris.Context, err error) (*RespJson, bool) {
	if bqErr, ok := err.(*errors2.BadRequestError); ok {
		return customError2Response(bqErr.CustomErrorAdapter), true
	}

	return nil, false
}

func customConflictHandler(ctx iris.Context, err error) (*RespJson, bool) {
	if bqErr, ok := err.(*errors2.ConflictError); ok {
		return customError2Response(bqErr.CustomErrorAdapter), true
	}

	return nil, false
}

func defaultErrorHandler(ctx iris.Context, err error) (*RespJson, bool) {
	return &RespJson{
		Status: http.StatusInternalServerError,
		Msg:    err.Error(),
	}, true
}
