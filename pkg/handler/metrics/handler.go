package metrics

import (
	"fmt"

	"github.com/huhenry/hej/pkg/servicegraph"

	micro "github.com/huhenry/hej/pkg/microapp"

	"github.com/huhenry/hej/pkg/common/app"
	"github.com/huhenry/hej/pkg/prometheus"

	"github.com/kataras/iris/v12"

	"k8s.io/apimachinery/pkg/runtime/schema"

	customErrors "github.com/huhenry/hej/pkg/errors"
	"github.com/huhenry/hej/pkg/handler"
	"github.com/huhenry/hej/pkg/log"
	"github.com/huhenry/hej/pkg/metrics"
	"github.com/huhenry/hej/pkg/multiCluster"
)

var metricsLogger = log.RegisterScope("handler.metrics")

const (
	ServiceMeshGroup   = "microservices.troila.com"
	ServiceMeshVersion = "v1beta1"
	ServiceMeshKind    = "ServiceMesh"

	DefaultInstallNamespace = "istio-system"
	DefaultTag              = "1.6.0"
	DefaultProfile          = "default"

	JaegerPort = "5066"
)

var ServiceMeshGVK = &schema.GroupVersionKind{
	Group:   ServiceMeshGroup,
	Version: ServiceMeshVersion,
	Kind:    ServiceMeshKind,
}

func GetMetrics(mgr multiCluster.Manager, ctx iris.Context) {

	appCtx := handler.ExtractAppContext(ctx)
	clusterName := appCtx.ClusterName
	appResources := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}
	promClient, err := prometheus.NewP8sClient(mgr, clusterName)
	if err != nil {
		metricsLogger.Errorf("prometheus Newclient err %v", err)
		msg := fmt.Sprintf("prometheus connection failed : %s", err)
		handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, msg)
		return

	}

	basicOptions := metrics.MetricsBasicQueryOptions{P8sAPI: promClient.Api}

	metricsOpts := &metrics.MetricsQueryOptions{MetricsBasicQueryOptions: basicOptions, IsDetails: true, AppResources: appResources}
	if err := metricsOpts.Parse(ctx, appCtx.KubeNamespace); err != nil {
		metricsLogger.Errorf("metricsOpts parse err %s", err)
		handler.ResponseErr(ctx, err)
		return
	}

	if err := metricsOpts.ParseForService(ctx); err != nil {
		metricsLogger.Errorf("metricsOpts parse err %s", err)
		handler.ResponseErr(ctx, err)
		return
	}
	microservice, err := micro.MicroService().FetchMicroservice(appResources, metricsOpts.Service)
	if err != nil {
		metricsLogger.Errorf("microservice get err %s", err)
		handler.ResponseErr(ctx, err)
		return
	}

	var result map[string]interface{} = make(map[string]interface{}, 0)

	if microservice.Spec.ServiceEntry != nil && microservice.Spec.ServiceEntry.Hosts != nil && len(microservice.Spec.ServiceEntry.Hosts) > 0 {
		metricsOpts.Service = microservice.Spec.ServiceEntry.Hosts[0]
		metricsOpts.IsServiceEntry = true
		//appProtocol = protocol.Parse(*microservice.AppProtocol)
	}

	if len(metricsOpts.Protocols) == 0 {
		appProtocol := ""
		switch {

		case microservice.HasGrpcAndHttp():
			appProtocol = "httpOrgrpc"
		case microservice.HasGrpc():
			appProtocol = "grpc"
		case microservice.IsHaveHTTP():
			appProtocol = "http"

		}

		mtrix, err := metrics.GetMetrics(ctx.Request().Context(), metricsOpts, appProtocol, metricsOpts.IsServiceEntry)
		if err != nil {
			metricsLogger.Errorf("GetMetrics err %s", err)
			handler.ResponseErr(ctx, err)
			return
		}

		if metrics.IsValideMetrics(mtrix, appProtocol) {

			result[appProtocol] = mtrix
		}

		if microservice.IsHaveTCP() {
			appProtocol = "tcp"
			mtrix, err := metrics.GetMetrics(ctx.Request().Context(), metricsOpts, appProtocol, metricsOpts.IsServiceEntry)
			if err != nil {
				metricsLogger.Errorf("GetMetrics err %s", err)
				handler.ResponseErr(ctx, err)
				return
			}
			result[appProtocol] = mtrix

		}

		handler.ResponseOk(ctx, result)
		return

		//

	}

	for _, appProtocol := range metricsOpts.Protocols {
		restMetrics, err := metrics.GetMetrics(ctx.Request().Context(), metricsOpts, appProtocol, metricsOpts.IsServiceEntry)
		if err != nil {
			metricsLogger.Errorf("GetMetrics err %s", err)
			handler.ResponseErr(ctx, err)
			return
		}
		if metrics.IsValideMetrics(restMetrics, appProtocol) {

			result[appProtocol] = restMetrics
		}

	}

	handler.ResponseOk(ctx, result)
	return
}

func GetAppMetrics(mgr multiCluster.Manager, ctx iris.Context) {

	appCtx := handler.ExtractAppContext(ctx)
	clusterName := appCtx.ClusterName
	appResources := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}
	promClient, err := prometheus.NewP8sClient(mgr, clusterName)
	if err != nil {
		metricsLogger.Errorf("prometheus Newclient err %v", err)
		msg := fmt.Sprintf("prometheus connection failed : %s", err)
		handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, msg)
		return

	}

	basicOptions := metrics.MetricsBasicQueryOptions{P8sAPI: promClient.Api}

	metricsOpts := &metrics.MetricsQueryOptions{MetricsBasicQueryOptions: basicOptions, IsDetails: true, AppResources: appResources}
	if err := metricsOpts.Parse(ctx, appCtx.KubeNamespace); err != nil {
		metricsLogger.Errorf("metricsOpts parse err %s", err)
		handler.ResponseErr(ctx, err)
		return
	}

	if err := metricsOpts.ParseForVersionedAPP(ctx); err != nil {
		metricsLogger.Errorf("metricsOpts parse err %s", err)
		handler.ResponseErr(ctx, err)
		return
	}
	if metricsOpts.App != "unknown" {

		microservice, err := micro.MicroServiceEntry().Get(appResources, metricsOpts.App, "")
		if err != nil {
			metricsLogger.Errorf("microservice get err %s", err)
			handler.ResponseErr(ctx, err)
			return
		}
		//appProtocol := protocol.Parse(metricsOpts.AppProtocol)

		if microservice.ServiceEntry != nil && microservice.ServiceEntry.Hosts != nil && len(microservice.ServiceEntry.Hosts) > 0 {
			metricsOpts.Service = microservice.ServiceEntry.Hosts[0]
			metricsOpts.IsServiceEntry = true
			//appProtocol = protocol.Parse(*microservice.AppProtocol)
		}

	} else {
		metricsOpts.Namespace = appCtx.KubeNamespace
	}

	var result map[string]interface{} = make(map[string]interface{}, 0)

	for _, appProtocol := range metricsOpts.Protocols {
		restMetrics, err := metrics.GetMetrics(ctx.Request().Context(), metricsOpts, appProtocol, metricsOpts.IsServiceEntry)
		if err != nil {
			metricsLogger.Errorf("GetMetrics for protocol %s err: %s", appProtocol, err)
			//handler.ResponseErr(ctx, err)
			continue
		}
		if metrics.IsValideMetrics(restMetrics, appProtocol) {

			result[appProtocol] = restMetrics
		}

	}

	handler.ResponseOk(ctx, result)
	return
}

func GetEdgeMetrics(mgr multiCluster.Manager, ctx iris.Context) {

	appCtx := handler.ExtractAppContext(ctx)
	clusterName := appCtx.ClusterName
	appResources := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}
	promClient, err := prometheus.NewP8sClient(mgr, clusterName)
	if err != nil {
		metricsLogger.Errorf("prometheus Newclient err %v", err)
		msg := fmt.Sprintf("prometheus connection failed : %s", err)
		handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, msg)
		return

	}

	basicOptions := metrics.MetricsBasicQueryOptions{P8sAPI: promClient.Api}
	if err := basicOptions.Parse(ctx, appCtx.KubeNamespace); err != nil {
		metricsLogger.Errorf("metricsOpts parse err %s", err)
		handler.ResponseErr(ctx, err)
		return
	}

	edgeOpts := &metrics.EdgeMetricsQueryOptions{MetricsBasicQueryOptions: basicOptions, AppResources: appResources}

	if err := edgeOpts.Parse(ctx); err != nil {
		metricsLogger.Errorf("edge metricsOpts parse err %s", err)
		handler.ResponseErr(ctx, err)
		return
	}
	var result map[string]interface{} = make(map[string]interface{}, 0)

	if edgeOpts.TargetType == servicegraph.NodeTypeServiceEntry {
		targetMicroservice, err := micro.MicroServiceEntry().Get(appResources, edgeOpts.TargetService, "")
		if err != nil {
			metricsLogger.Errorf("microservice get err %s", err)
			handler.ResponseErr(ctx, err)
			return
		}
		if targetMicroservice.ServiceEntry != nil && targetMicroservice.ServiceEntry.Hosts != nil && len(targetMicroservice.ServiceEntry.Hosts) > 0 {
			edgeOpts.TargetService = targetMicroservice.ServiceEntry.Hosts[0]
			edgeOpts.TargetNamespace = ""

		}
	}

	for _, appProtocol := range edgeOpts.Protocols {
		restMetrics, err := metrics.GetMetrics(ctx.Request().Context(), edgeOpts, appProtocol, false)
		if err != nil {
			metricsLogger.Errorf("GetMetrics err %s", err)
			handler.ResponseErr(ctx, err)
			return
		}

		if metrics.IsValideMetrics(restMetrics, appProtocol) {

			result[appProtocol] = restMetrics
		}

	}

	handler.ResponseOk(ctx, result)
	return
}
