package graph

import (
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/huhenry/hej/pkg/prometheus"

	customErrors "github.com/huhenry/hej/pkg/errors"
	"github.com/kataras/iris/v12"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/huhenry/hej/pkg/common"
	"github.com/huhenry/hej/pkg/handler"
	"github.com/huhenry/hej/pkg/log"
	"github.com/huhenry/hej/pkg/multiCluster"
	graph "github.com/huhenry/hej/pkg/servicegraph"
)

var servicegraphLogger = log.RegisterScope("handler.metrics")

const (

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

func GetGraphs(mgr multiCluster.Manager, ctx iris.Context) {

	appCtx := handler.ExtractAppContext(ctx)
	clusterName := appCtx.ClusterName
	microapp := ctx.Params().GetString("application")
	k8sClient, err := mgr.Client(clusterName)
	if err != nil {

		servicegraphLogger.Errorf("mgr.Client %S", err)
		msg := fmt.Sprintf("集群连接失败 : %s", err)
		handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, msg)

		return
	}

	promClient, err := prometheus.NewP8sClient(mgr, clusterName)
	if err != nil {
		servicegraphLogger.Errorf("prometheus Newclient err %v", err)
		msg := fmt.Sprintf("prometheus 连接失败 : %s", err)
		handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, msg)
		return

	}

	namespace := appCtx.KubeNamespace
	startTime, err := strconv.Atoi(ctx.URLParam(PathParameterStartTime))
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}
	endTime, err := strconv.Atoi(ctx.URLParam(PathParameterEndTime))
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}
	endTime = common.ParseEndTime(endTime)
	microAppClient, err := mgr.DynamicClient(clusterName, MicroAppGVK)
	if err != nil {
		servicegraphLogger.Errorf("DynamicClient %s", err)

		handler.ResponseErr(ctx, err)
		return

	}
	microApp, err := microAppClient.Namespace(namespace).Get(ctx.Request().Context(), microapp, metav1.GetOptions{})
	if err != nil {
		servicegraphLogger.Errorf("DynamicClient %s", err)

		handler.ResponseErr(ctx, err)
		return
	}
	microAppCreateTime := microApp.GetCreationTimestamp().Unix()
	if microAppCreateTime > int64(startTime) {
		startTime = int(microAppCreateTime)
	}
	if microAppCreateTime > int64(endTime) {
		endTime = int(microAppCreateTime + 3600)
	}
	microserviceClient, err := mgr.DynamicClient(clusterName, MicroServiceGVK)
	if err != nil {
		servicegraphLogger.Errorf("DynamicClient %s", err)

		handler.ResponseErr(ctx, err)
		return

	}
	var canaryGVK = &schema.GroupVersionKind{
		Group:   common.ServiceMeshGroup,
		Version: common.ServiceMeshVersion,
		Kind:    common.CanaryKind,
	}

	canaryClient, err := mgr.DynamicClient(clusterName, canaryGVK)
	if err != nil {
		servicegraphLogger.Errorf("DynamicClient %+v, %s", *canaryGVK, err)

		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))
		return

	}
	istioClient, _ := mgr.IstioClient(clusterName)
	result, err := graph.GetGraph(ctx.Request().Context(), k8sClient, microserviceClient, canaryClient, promClient, namespace, microapp, startTime, endTime, istioClient)
	if err != nil {
		servicegraphLogger.Errorf("fetch graph err %v", err)
		//fmt.Printf("servicegraph get graph error %s", err)
		handler.ResponseErr(ctx, err)
		return
	}
	handler.ResponseOk(ctx, result)
}
