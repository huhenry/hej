package handler

import (
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/huhenry/hej/pkg/common/page"

	"github.com/huhenry/hej/pkg/prometheusmetrics"

	"github.com/huhenry/hej/pkg/define"

	log "github.com/sirupsen/logrus"

	"github.com/kataras/iris/v12"
	//"github.com/kataras/iris/v12/v12/context"
	"github.com/kataras/iris/v12/context"
)

const (
	ResponseStatusOk      = "OK"
	DNS1123LabelMaxLength = 63 // Public for testing only.
	dns1123LabelFmt       = "[a-zA-Z0-9](?:[-a-zA-Z0-9]*[a-zA-Z0-9])?"
)

var dns1123LabelRegexp = regexp.MustCompile("^" + dns1123LabelFmt + "$")

// IsDNS1123Label tests for a string that conforms to the definition of a label in
// DNS (RFC 1123).
func IsDNS1123Label(value string) bool {
	return len(value) <= DNS1123LabelMaxLength && dns1123LabelRegexp.MatchString(value)
}

type HandFunc = context.Handler // func(ctx iris.Context)

type RespJson struct {
	Status int         `json:"code"`
	Msg    string      `json:"message"`
	Data   interface{} `json:"data,omitempty"`
	Detail string      `json:"detail,omitempty""`
}

func NewRespJson() *RespJson {
	return &RespJson{
		Status: define.ST_OK,
	}
}

type RespMsgListJson struct {
	Status int         `json:"code"`
	Msg    []string    `json:"message"`
	Data   interface{} `json:"data,omitempty"`
	Detail string      `json:"detail,omitempty""`
}

// //////////////////////////////////////////////////////
func GetNulString(ctx iris.Context, k string) string {
	return ctx.FormValue(k)
}
func GetString(ctx iris.Context, k string) string {

	data := ctx.FormValue(k)
	if data == "" {
		Response(ctx, define.ST_ARGS_ERROR, k+"is empty")
		return ""
	}
	return data
}

func GetInt(ctx iris.Context, k string) int {
	data, err := strconv.Atoi(ctx.FormValue(k))
	if err != nil {
		Response(ctx, define.ST_ARGS_ERROR, k+"is invilid")
		return -1
	}
	return data
}
func GetInt64(ctx iris.Context, k string) int64 {
	data, err := strconv.ParseInt(ctx.FormValue(k), 10, 64)
	if err != nil {
		Response(ctx, define.ST_ARGS_ERROR, k+"is invalid")
		return -1
	}
	return data
}

// //////////////////////////////////////////////
// access log- request
func RequestLog(ctx iris.Context) {
	promtimer := prometheusmetrics.GetAPIProcessingTimePrometheusTimer(ctx.Path())
	defer promtimer.ObserveDuration()
	var params interface{}
	params = ctx.FormValues()
	user := ctx.Values().Get(define.ReqUserKey)
	begin := time.Now()
	ctx.Values().Set(define.CosTimeKey, begin.UnixNano())
	if ctx.Path() != "/api/v1/healthz" {

		log.WithField("serino", begin.UnixNano()).WithField("path", ctx.Path()).WithField("peer", ctx.RemoteAddr()).WithField("user", user).Info(params)
	}
	ctx.Next()
}

// access log -response
func responseLog(ctx iris.Context, respdata interface{}) {
	var costtime int64
	begin, _ := ctx.Values().GetInt64(define.CosTimeKey)

	costtime = time.Now().UnixNano() - begin
	peerin := ctx.RemoteAddr()
	user := ""
	userCtx := ctx.Values().Get(define.UserContextKey)
	if userCtx != nil {
		if uCtx, ok := userCtx.(UserContext); ok {
			user = uCtx.Name
		}
	}
	respstatus := ctx.Values().Get(define.CtxRespStsKey)

	if ctx.Method() == http.MethodGet {
		// hidden request about get request
		if respstatus == ResponseStatusOk {
			respdata = "OK"
		}
		log.WithField("path", ctx.Path()).
			WithField("serino", begin).
			WithField("peer", peerin).
			WithField("user", user).
			WithField("remote_ip", ctx.Request().RemoteAddr).
			WithField("costtime", costtime).Debug(respdata)

	} else {
		log.WithField("path", ctx.Path()).
			WithField("serino", begin).
			WithField("peer", peerin).
			WithField("user", user).
			WithField("remote_ip", ctx.Request().RemoteAddr).
			WithField("costtime", costtime).Info(respdata)
	}
}

func ExtractAppContext(ctx iris.Context) *AppClusterContext {
	return ctx.Values().Get(define.AppContextKey).(*AppClusterContext)
}

func TryGetCluster(ctx iris.Context) string {
	value := ctx.Values().Get(define.AppContextKey)
	if value != nil {
		if ctx, ok := value.(*AppClusterContext); ok {
			return ctx.ClusterName
		}
	}

	return ""
}

func ExtractUserContext(ctx iris.Context) *UserContext {
	return ctx.Values().Get(define.UserContextKey).(*UserContext)
}

func ExtractQueryParam(ctx iris.Context) *page.QueryParam {
	pageNo := ctx.URLParamInt64Default("pageNo", 1)
	if pageNo <= 0 {
		pageNo = 1
	}
	pageSize := ctx.URLParamInt64Default("pageSize", 10)
	if pageSize <= 0 {
		pageSize = 10
	}
	sortby := ctx.URLParamDefault("sortby", "")
	name := ctx.URLParamDefault("name", "")

	return &page.QueryParam{
		PageNo:   pageNo,
		PageSize: pageSize,
		Sortby:   sortby,
		Name:     name,
	}
}
