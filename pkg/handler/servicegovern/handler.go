package servicegovern

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/huhenry/hej/pkg/common"
	"github.com/huhenry/hej/pkg/common/page"
	"github.com/huhenry/hej/pkg/define"
	customErrors "github.com/huhenry/hej/pkg/errors"
	"github.com/huhenry/hej/pkg/handler"
	"github.com/huhenry/hej/pkg/log"
	"github.com/huhenry/hej/pkg/multiCluster"
	"github.com/huhenry/hej/pkg/prometheus"
	serviceGovern "github.com/huhenry/hej/pkg/serviceGovern"
	"github.com/kataras/iris/v12"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var logger = log.RegisterScope("handler.serviceGovern")

const (
	PathParameterSourceService  = "source_service"
	PathParameterTargetWorkload = "destination_workload"
	PathParameterTargetService  = "destination_service"
	PathParameterProtocol       = "protocol"
	PathParameterEvent          = "event_type"

	ServiceType = "service"
	AppType     = "app"
	GatewayType = "gateway"
	// PathParameterNamespace is the path parameter name of namespace
	PathParameterNamespace = "namespace"
	// path parameterStart time
	PathParameterStartTime = "start_time"
	// PathParameterEndTime is the path parameter end_time
	PathParameterEndTime = "end_time"
	PathParameterStep    = "step"

	DefaultInstallNamespace = "istio-system"
	DefaultTag              = "1.6.0"
	DefaultProfile          = "default"

	JaegerPort = "5066"
)

var ServiceMeshGVK = &schema.GroupVersionKind{
	Group:   common.ServiceMeshGroup,
	Version: common.ServiceMeshVersion,
	Kind:    common.ServiceMeshKind,
}

var MicroServiceGVK = &schema.GroupVersionKind{
	Group:   common.ServiceMeshGroup,
	Version: common.ServiceMeshVersion,
	Kind:    common.MicroServiceKind,
}

var MicroAppGVK = &schema.GroupVersionKind{
	Group:   common.ServiceMeshGroup,
	Version: common.ServiceMeshVersion,
	Kind:    common.MicroAppKind,
}

var EventTypeCn = map[string]string{
	"TrafficLimit":        "服务限流",
	"CircuitBreaker":      "服务熔断",
	"DelayFaultInjection": "故障注入(时延)",
	"AbortFaultInjection": "故障注入(中断)",
}

var SourceServiceTypeCn = map[string]string{
	"external": "(应用外服务)",
	"internal": "",
}
var DestinationServiceTypeCn = map[string]string{
	"external": "(外部服务)",
	"internal": "",
}

func ParseOptions(mgr multiCluster.Manager, ctx iris.Context) (*serviceGovern.ServiceEventQueryOptions, error) {

	appCtx := handler.ExtractAppContext(ctx)
	clusterName := appCtx.ClusterName
	microapp := ctx.Params().GetString("application")

	namespace := appCtx.KubeNamespace
	options, err := ParseParams(ctx)
	if err != nil {
		logger.Errorf("ParseOptions err %s", err)
		//msg := fmt.Sprintf("开始时间或结束时间格式不正确 : %s", err)
		//handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, msg)

		return nil, fmt.Errorf("开始时间或结束时间格式不正确 : %s", err)
	}
	options.MicroApp = microapp
	options.Namespace = namespace

	promClient, err := prometheus.NewP8sClient(mgr, clusterName)
	if err != nil {
		logger.Errorf("prometheus Newclient err %v", err)
		//msg := fmt.Sprintf
		//handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, msg)
		return nil, fmt.Errorf("prometheus 连接失败 : %s", err)

	}
	options.P8sClient = promClient

	microAppClient, err := mgr.DynamicClient(clusterName, MicroAppGVK)
	if err != nil {
		logger.Errorf("DynamicClient %s", err)

		//handler.ResponseErr(ctx, err)
		return nil, fmt.Errorf("微服务应用资源不存在 : %s", err)

	}
	microApp, err := microAppClient.Namespace(namespace).Get(ctx.Request().Context(), microapp, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("fetchMicroAPP error: %s", err)

		return nil, fmt.Errorf("微服务应用[%s]不存在 : %s", microapp, err)
	}
	microAppCreateTime := microApp.GetCreationTimestamp().Unix()
	if microAppCreateTime > int64(options.StartTime) {
		options.StartTime = int64(microAppCreateTime)
	}
	if microAppCreateTime > int64(options.EndTime) {
		options.EndTime = int64(microAppCreateTime + 3600)
	}

	microserviceClient, err := mgr.DynamicClient(clusterName, MicroServiceGVK)
	if err != nil {
		logger.Errorf("DynamicClient %s", err)

		return nil, fmt.Errorf("微服务资源不存在 : %s", err)

	}
	options.MicroserviceClient = microserviceClient

	return options, nil
}

// Parse restful request to ServiceEventQueryOptions
func ParseParams(ctx iris.Context) (*serviceGovern.ServiceEventQueryOptions, error) {
	sourceService := ctx.URLParam(PathParameterSourceService)
	targetWorkload := ctx.URLParam(PathParameterTargetWorkload)
	targetService := ctx.URLParam(PathParameterTargetService)
	protocol := ctx.URLParam(PathParameterProtocol)
	eventType := ctx.URLParam(PathParameterEvent)

	startTime, err := strconv.Atoi(ctx.URLParam(PathParameterStartTime))
	if err != nil {
		return nil, err
	}

	endTime, err := strconv.Atoi(ctx.URLParam(PathParameterEndTime))
	if err != nil {
		return nil, err
	}
	endTime = common.ParseEndTime(endTime)

	options := &serviceGovern.ServiceEventQueryOptions{
		SourceService:       sourceService,
		DestinationService:  targetService,
		DestinationWorkload: targetWorkload,
		StartTime:           int64(startTime),
		EndTime:             int64(endTime),
		Protocol:            protocol,
	}
	if eventType != "" {
		options.Event = serviceGovern.EventType(eventType)
	}
	return options, nil
}

func FetchServiceGovernEvents(mgr multiCluster.Manager, ctx iris.Context) {

	opts, err := ParseOptions(mgr, ctx)

	paramQuery := handler.ExtractQueryParam(ctx)
	if err != nil {
		handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, err.Error())
		return
	}
	result, err := serviceGovern.FetchServiceEvent(ctx.Request().Context(), opts)
	if err != nil {
		logger.Errorf("fetch ServiceEvent err %s", err)
		//fmt.Printf("servicegraph get graph error %s", err)
		handler.ResponseErr(ctx, err)
		return
	}
	sort.Slice(result, func(i, j int) bool {
		r := result[i].EventAt > result[j].EventAt
		if paramQuery.Sortby == define.SortByCreateTime {
			r = !r
		}

		return r
	})
	data := make([]interface{}, 0)
	for i := range result {
		dto := &result[i]
		data = append(data, dto)
	}
	handler.ResponseOk(ctx, page.PageInfo(data, paramQuery))
	//handler.ResponseOk(ctx, result)
}

func FetchSourceServiceLabels(mgr multiCluster.Manager, ctx iris.Context) {

	opts, err := ParseOptions(mgr, ctx)
	if err != nil {
		handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, err.Error())
		return
	}
	labels := serviceGovern.FetchLabels(ctx.Request().Context(), "source_app", opts)

	handler.ResponseOk(ctx, labels)
}
func FetchDestServiceLabels(mgr multiCluster.Manager, ctx iris.Context) {

	opts, err := ParseOptions(mgr, ctx)
	if err != nil {
		handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, err.Error())
		return
	}
	labels := serviceGovern.FetchLabels(ctx.Request().Context(), "destination_service_name", opts)

	handler.ResponseOk(ctx, labels)
}
func FetchDestWorkloadLabels(mgr multiCluster.Manager, ctx iris.Context) {

	opts, err := ParseOptions(mgr, ctx)
	if err != nil {
		handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, err.Error())
		return
	}
	labels := serviceGovern.FetchLabels(ctx.Request().Context(), "destination_workload", opts)

	handler.ResponseOk(ctx, labels)
}

func ExportServiceGoverned(mgr multiCluster.Manager, ctx iris.Context) {
	opts, err := ParseOptions(mgr, ctx)

	paramQuery := handler.ExtractQueryParam(ctx)
	if err != nil {
		handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, err.Error())
		return
	}
	result, err := serviceGovern.FetchServiceEvent(ctx.Request().Context(), opts)
	if err != nil {
		logger.Errorf("fetch ServiceEvent err %s", err)
		//fmt.Printf("servicegraph get graph error %s", err)
		handler.ResponseErr(ctx, err)
		return
	}
	sort.Slice(result, func(i, j int) bool {
		r := result[i].EventAt > result[j].EventAt
		if paramQuery.Sortby == define.SortByCreateTime {
			r = !r
		}

		return r
	})
	b := &bytes.Buffer{}
	w := csv.NewWriter(b)
	// 写入UTF-8 BOM，防止乱码
	b.WriteString("\xEF\xBB\xBF")
	w.Write([]string{"事件类型", "请求服务", "作用服务", "作用工作负载", "版本", "服务协议", "响应码", "事件时间"})
	for _, record := range result {
		record := convertArray(record)
		if err := w.Write(record); err != nil {
			logger.Error("error writing record to csv:", err)
		}
	}

	// Write any buffered data to the underlying writer (standard output).
	w.Flush()

	if err := w.Error(); err != nil {
		logger.Error("Write any buffered data", err)
	}

	//ctx.Header("Content-Description", "File Transfer")
	ctx.Header("Content-type", "text/csv")
	ctx.Header("Content-Disposition",
		`attachment; filename="servicegovern.csv"`)
	ctx.Write(b.Bytes())

}

func convertArray(value *serviceGovern.ServiceGovernedEvent) []string {

	var canary string
	if value.IsCanary {
		canary = "（灰度版本）"
	}
	eventType := EventTypeCn[string(value.Event)]
	sourceService := value.SourceService + SourceServiceTypeCn[string(value.SourceServiceType)]
	destinationService := value.DestinationService + SourceServiceTypeCn[string(value.DestinationServiceType)]
	destinationWorkload := value.DestinationWorkload
	version := value.DestinationVersion + canary
	protocol := value.Protocol
	resCode := value.ResponseCode

	eventat, err := strconv.ParseInt(value.EventAt, 10, 64)
	if err != nil {
		logger.Errorf("serviceGovern EventTime convrt err %s", err)

	}
	eventTime := time.Unix(eventat, 0).Format("2006-01-02 15:04:05")

	all := fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s", eventType, sourceService, destinationService, destinationWorkload, version, protocol, resCode, eventTime)
	return strings.Split(all, ",")

}
