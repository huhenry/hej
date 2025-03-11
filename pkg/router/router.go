package router

import (
	"github.com/huhenry/hej/pkg/handler"
	"github.com/huhenry/hej/pkg/handler/auth"
	"github.com/huhenry/hej/pkg/handler/canary"
	"github.com/huhenry/hej/pkg/handler/graph"
	"github.com/huhenry/hej/pkg/handler/installation"
	"github.com/huhenry/hej/pkg/handler/installation/bookinfo"
	"github.com/huhenry/hej/pkg/handler/license"
	"github.com/huhenry/hej/pkg/handler/metrics"
	workloadMetrics "github.com/huhenry/hej/pkg/handler/metrics/workload"
	"github.com/huhenry/hej/pkg/handler/microapp"
	"github.com/huhenry/hej/pkg/handler/servicegovern"
	"github.com/huhenry/hej/pkg/handler/traffic"
	"github.com/huhenry/hej/pkg/multiCluster"
	iris "github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/context"
	"github.com/kataras/iris/v12/middleware/pprof"
)

// QueryContextHandler is the interface for restfuul handler(restful.Request,restful.Response)
type MultiClusterHandler func(mgr multiCluster.Manager, ctx iris.Context)

// RegisterHandler is the wrapper to combine the RESTfulContextHandler with our serviceprovider object
func RegisterMultiClusterHandler(mgr multiCluster.Manager, handler MultiClusterHandler) context.Handler {
	return func(ctx iris.Context) {
		handler(mgr, ctx)
	}
}

// //////////////////////////////
// router
func (a *API) InitRouter() *API {
	//init middleware here
	a.SetMiddleware(handler.RequestLog)

	// check session
	//a.SetMiddleware(middleware.CheckToken)
	//a.SetDone(/*请求收尾处理函数*/)

	rootAPI := a.Group("/api/v1")

	p := pprof.New()

	mc := auth.Handler(auth.MC)
	mr := auth.Handler(auth.MR)
	mu := auth.Handler(auth.MU)

	clusterManagerRoot := rootAPI.Group("/clusters/{cluster}", handler.VerifyToken, license.Handler())

	appClusterRoot := rootAPI.Group("/apps/{app}/clusters/{cluster}", handler.VerifyToken, license.Handler(), handler.ExtractAppCluster)

	{
		appClusterRoot.Get("/resources/available/services", auth.Handler(auth.SR), microapp.GetAvailableServices)
	}

	{

		appClusterRoot.Post("/applications", mc, microapp.CreateApplication)
		appClusterRoot.Get("/applications/{name}", mr, microapp.GetApplication)
		appClusterRoot.Put("/applications/{name}", mu, microapp.UpdateApplication)
		appClusterRoot.Delete("/applications/{name}", auth.Handler(auth.MD, auth.SD, auth.DD), microapp.DeleteApplication)
		appClusterRoot.Get("/applications", mr, microapp.ListApplication)
		appClusterRoot.Get("/microapps", mr, microapp.ListApplicationNames)

		appClusterRoot.Post("/applications/{application}/microservices", auth.Handler(auth.MU, auth.SU, auth.DU), microapp.CreateMicroService)
		appClusterRoot.Post("/applications/{application}/microservice_batch", auth.Handler(auth.MU, auth.SU, auth.DU), microapp.BatchCreateMicroService)
		appClusterRoot.Get("/applications/{application}/microservices", mr, microapp.ListMicroService)
		appClusterRoot.Delete("/applications/{application}/microservices/{name}", auth.Handler(auth.MU), microapp.DeleteMicroService)
		appClusterRoot.Get("/applications/{application}/servicenames", mr, microapp.ListApplicationServiceNames)
		appClusterRoot.Get("/applications/{application}/microservices/{name}/workload", mr, RegisterMultiClusterHandler(a.Manager, microapp.GetWorkloadContainers))
		appClusterRoot.Get("/applications/{application}/microservices/{name}/availableworkloads", microapp.GetAvailableWorkloads)

		appClusterRoot.Post("/applications/{application}/serviceEntries", auth.Handler(auth.MU), RegisterMultiClusterHandler(a.Manager, microapp.CreateMicroServiceEntry))
		appClusterRoot.Put("/applications/{application}/serviceEntries", auth.Handler(auth.MU), RegisterMultiClusterHandler(a.Manager, microapp.UpdateMicroServiceEntry))
		appClusterRoot.Get("/applications/{application}/serviceEntries", mr, microapp.ListMicroServiceEntry)
		appClusterRoot.Get("/applications/{application}/serviceEntries/{name}", mr, microapp.GetMicroServiceEntry)
		appClusterRoot.Delete("/applications/{application}/serviceEntries/{name}", auth.Handler(auth.MU), RegisterMultiClusterHandler(a.Manager, microapp.DeleteMicroServiceEntry))

		appClusterRoot.Get("/applications/{application}/microservices/{name}/policy", mr, traffic.GetPolicy)
		appClusterRoot.Put("/applications/{application}/microservices/{name}/policy", auth.Handler(auth.MU, auth.SU), traffic.SetPolicy)
		appClusterRoot.Post("/applications/{application}/gateways", mu, microapp.CreateGateway)
		appClusterRoot.Post("/applications/{application}/gateway_batch", mu, RegisterMultiClusterHandler(a.Manager, microapp.BatchCreateGateway))
		appClusterRoot.Get("/applications/{application}/gateways", mr, microapp.ListGateway)
		appClusterRoot.Delete("/applications/{application}/gateways/{name}", mu, RegisterMultiClusterHandler(a.Manager, microapp.DeleteGateway))

		appClusterRoot.Get("/metrics/service/{service}", RegisterMultiClusterHandler(a.Manager, metrics.GetMetrics))
		appClusterRoot.Get("/metrics/unknown/{nodeType}", RegisterMultiClusterHandler(a.Manager, metrics.GetMetrics))
		appClusterRoot.Get("/metrics/edge", RegisterMultiClusterHandler(a.Manager, metrics.GetEdgeMetrics))
		appClusterRoot.Get("/metrics/app/{name}/{version}", RegisterMultiClusterHandler(a.Manager, metrics.GetAppMetrics))
		appClusterRoot.Get("/metrics/workload/{name}", RegisterMultiClusterHandler(a.Manager, workloadMetrics.GetMetrics))
		appClusterRoot.Get("/graphs", RegisterMultiClusterHandler(a.Manager, graph.GetGraphs))
		appClusterRoot.Get("/applications/{application}/graphs", RegisterMultiClusterHandler(a.Manager, graph.GetGraphs))
		appClusterRoot.Get("/applications/{application}/serviceevent", RegisterMultiClusterHandler(a.Manager, servicegovern.FetchServiceGovernEvents))
		appClusterRoot.Get("/applications/{application}/serviceevent/export", RegisterMultiClusterHandler(a.Manager, servicegovern.ExportServiceGoverned))
		appClusterRoot.Get("/applications/{application}/sourceservice/list", RegisterMultiClusterHandler(a.Manager, servicegovern.FetchSourceServiceLabels))
		appClusterRoot.Get("/applications/{application}/destservice/list", RegisterMultiClusterHandler(a.Manager, servicegovern.FetchDestServiceLabels))
		appClusterRoot.Get("/applications/{application}/destworkload/list", RegisterMultiClusterHandler(a.Manager, servicegovern.FetchDestWorkloadLabels))
		//
		appClusterRoot.Post("/demo",
			auth.Handler(auth.MC, auth.MU, auth.SC, auth.DC),
			bookinfo.InstallHandler(a.Manager))

		appClusterRoot.Post("/applications/{application}/canary", auth.HandlerWithMsg("权限不足。需拥有无状态负载的创建和编辑权限才可进行当前操作。", auth.CC, auth.DC, auth.DU), RegisterMultiClusterHandler(a.Manager, canary.CreateCanary))
		appClusterRoot.Get("/applications/{application}/canaries", RegisterMultiClusterHandler(a.Manager, canary.ListCanary))
		appClusterRoot.Get("/canaries", RegisterMultiClusterHandler(a.Manager, canary.ListCanary))
		appClusterRoot.Get("/applications/{application}/canary/{canary}", RegisterMultiClusterHandler(a.Manager, canary.GetCanaryDetail))
		appClusterRoot.Get("/applications/{application}/canary/{canary}/metricscount", RegisterMultiClusterHandler(a.Manager, canary.GetCanaryMetriceSummary))
		appClusterRoot.Get("/applications/{application}/canary/{canary}/metricsweight", RegisterMultiClusterHandler(a.Manager, canary.GetCanaryMetriceWeight))

		appClusterRoot.Get("/applications/{application}/canary/{canary}/metrics", RegisterMultiClusterHandler(a.Manager, canary.GetCanaryMetrics))
		appClusterRoot.Delete("/applications/{application}/canary/{canary}", auth.HandlerWithMsg("权限不足。需拥有无状态负载的删除权限才可进行当前操作。", auth.CD, auth.DD), RegisterMultiClusterHandler(a.Manager, canary.DeleteCanary))
		//
		appClusterRoot.Get("/applications/{application}/canary/{canary}/policy", RegisterMultiClusterHandler(a.Manager, canary.GetCanaryPolicy))
		appClusterRoot.Put("/applications/{application}/canary/{canary}/policy", RegisterMultiClusterHandler(a.Manager, canary.UpdateCanaryPolicy))

		appClusterRoot.Put("/applications/{application}/canary/{canary}/takeoveralltraffic", RegisterMultiClusterHandler(a.Manager, canary.TakeOverAllTraffic))
		appClusterRoot.Put("/applications/{application}/canary/{canary}/gooffline", auth.HandlerWithMsg("权限不足。需拥有无状态负载的删除权限才可进行当前操作。", auth.CD, auth.DD), RegisterMultiClusterHandler(a.Manager, canary.GoOffline))
		appClusterRoot.Get("/applications/{application}/workload/{name}/validation", RegisterMultiClusterHandler(a.Manager, canary.WorkloadValidation))
		//
		appClusterRoot.Get("/applications/{application}/microservice/{name}/validation", RegisterMultiClusterHandler(a.Manager, canary.MicroServiceValidation))
		appClusterRoot.Get("/applications/{application}/microservice/{service}/availableworkload/{name}/validation", RegisterMultiClusterHandler(a.Manager, canary.AvailableWorkloadValidation))

		appClusterRoot.Get("/applications/{application}/workload/{workload}/pods", mr, microapp.ListWorkloadPods)

		appClusterRoot.Get("/healthz/{node_type}/{name}", mr, microapp.Healthz)
	}

	//global api demo
	{
		clusterManagerRoot.Post("/servicemesh/install", RegisterMultiClusterHandler(a.Manager, installation.Install))
		clusterManagerRoot.Get("/servicemesh/egress/status", RegisterMultiClusterHandler(a.Manager, installation.EgressStatus))
		clusterManagerRoot.Post("/servicemesh/uninstall", RegisterMultiClusterHandler(a.Manager, installation.Uninstall))
		clusterManagerRoot.Post("/servicemesh/egress/{operation}", RegisterMultiClusterHandler(a.Manager, installation.EgressEnable))
		clusterManagerRoot.Get("/servicemesh/status", RegisterMultiClusterHandler(a.Manager, installation.Status))

		rootAPI.Get("/healthz", handler.Healthz)
		rootAPI.Any("/debug/pprof", p)
		rootAPI.Any("/debug/pprof/{action}", p)
	}

	return a
}
