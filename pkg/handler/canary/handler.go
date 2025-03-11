package canary

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	"github.com/huhenry/hej/pkg/canary"
	microappcommon "github.com/huhenry/hej/pkg/microapp/common"

	microV1beta1 "github.com/huhenry/hej/pkg/microapp/v1beta1"

	k8serror "k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/labels"

	"github.com/huhenry/hej/pkg/define"
	"github.com/huhenry/hej/pkg/prometheus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/huhenry/hej/pkg/common"
	"github.com/huhenry/hej/pkg/handler"
	"github.com/huhenry/hej/pkg/handler/audit"
	"github.com/huhenry/hej/pkg/multiCluster"
	"github.com/kataras/iris/v12"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/huhenry/hej/pkg/log"

	customErrors "github.com/huhenry/hej/pkg/errors"
)

const (

	// DefaultStep for 60 miniseconds
	DefaultStep            = 60
	PathParameterStartTime = "start_time"
	// PathParameterEndTime is the path parameter end_time
	PathParameterEndTime = "end_time"
	PathParameterStep    = "step"

	PathParameterVersion    = "version"
	QueryParemeterQuantiles = "quantiles"

	DNS1123LabelMaxLength = 63
)

var canaryGVK = &schema.GroupVersionKind{
	Group:   common.ServiceMeshGroup,
	Version: common.ServiceMeshVersion,
	Kind:    common.CanaryKind,
}
var MicroServiceGVK = &schema.GroupVersionKind{
	Group:   common.ServiceMeshGroup,
	Version: common.ServiceMeshVersion,
	Kind:    common.MicroServiceKind,
}

var logger = log.RegisterScope("handler.canary")

type ValidResult struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message,omitempty"`
}

func valideCanary(canarySpec *canary.CanarySpec) error {
	if canarySpec.Services == nil || len(canarySpec.Services) == 0 {

		err := fmt.Errorf("灰度发布任务%s的灰度版本不可为空", canarySpec.Name)

		return err
	}
	for _, serviceRef := range canarySpec.Services {
		if !handler.IsDNS1123Label(serviceRef.Version) {
			err := fmt.Errorf("灰度发布任务%s的灰度版本%s不是有效的版本号", canarySpec.Name, serviceRef.Version)

			return err
		}

		if serviceRef.CopiedTarget != nil && *serviceRef.CopiedTarget.Replicas == 0 {
			err := fmt.Errorf("灰度发布任务%s工作负载副本数不可为0", canarySpec.Name)

			return err

		}
		if serviceRef.SelectedTarget != nil && *serviceRef.SelectedTarget.Replicas == 0 {
			err := fmt.Errorf("灰度发布任务%s工作负载副本数不可为0", canarySpec.Name)

			return err

		}
	}

	return ValidePolicy(canarySpec.Policy)

}

func CreateCanary(mgr multiCluster.Manager, ctx iris.Context) {

	canarySpec := &canary.CanarySpec{}
	err := ctx.ReadJSON(canarySpec)
	if err != nil {

		handler.RespondWithDetailedError(ctx, customErrors.BadParametersErr(err))
		return
	}

	if err := valideCanary(canarySpec); err != nil {

		//logger.Error(err)

		handler.RespondWithError(ctx, customErrors.StatusCodeHTTPRequestErrorCode, err)
		return
	}

	if valid := microServiceValidation(mgr, ctx, canarySpec.MicroService); valid != nil && !valid.Valid {

		handler.RespondWithError(ctx, customErrors.StatusCodeHTTPRequestErrorCode, fmt.Errorf(valid.Message))
		return
	}

	for _, serviceRef := range canarySpec.Services {
		if serviceRef.SelectedTarget != nil && serviceRef.SelectedTarget.Workload != nil {

			if valid := workloadValidation(mgr, ctx, serviceRef.SelectedTarget.Workload.Name, canarySpec.MicroService); valid != nil && !valid.Valid {
				handler.RespondWithError(ctx, customErrors.StatusCodeHTTPRequestErrorCode, fmt.Errorf(valid.Message))
				return
			}
		}

	}

	ms, err := fetchMicroservice(mgr, ctx, canarySpec.MicroService)
	if err != nil {
		handler.RespondWithError(ctx, customErrors.StatusCodeHTTPRequestErrorCode, fmt.Errorf("服务%s不存在", canarySpec.MicroService))
		return
	}

	if err := valideFailedStatus(ms, canarySpec); err != nil {
		handler.RespondWithError(ctx, customErrors.StatusCodeHTTPRequestErrorCode, fmt.Errorf(err.Error()))
		return
	}
	appCtx := handler.ExtractAppContext(ctx)
	userCtx := handler.ExtractUserContext(ctx)

	clusterName := appCtx.ClusterName
	namespace := appCtx.KubeNamespace
	microapp := ctx.Params().GetString("application")
	canarySpec.MicroApplication = microapp

	canaryClient, err := mgr.DynamicClient(clusterName, canaryGVK)
	if err != nil {
		logger.Errorf("Dynamic Client %+v, %s", *canaryGVK, err)

		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))
		return

	}

	if err := canary.CreateCanary(ctx.Request().Context(), canaryClient, namespace, userCtx.Name, appCtx.AppId, int64(appCtx.NamespaceId), *canarySpec); err != nil {

		logger.Errorf("Create Canary %+v, err %s", *canarySpec, err)

		handler.RespondWithDetailedError(ctx, customErrors.CustomClientErr("创建灰度任务失败！", err))
		return
	}

	handler.SendAudit(audit.ModuleCanary, audit.ActionCreate, canarySpec.Name, ctx)

	handler.ResponseOk(ctx, nil)

}

func valideFailedStatus(ms *microV1beta1.MicroService, cay *canary.CanarySpec) error {

	//udp check
	if _, isAllUdp := microappcommon.CovertPorts(ms.Spec.Ports); isAllUdp {
		return fmt.Errorf("灰度发布暂不支持UDP协议的服务")
	}

	//noHttp mirroing check
	if cay.CanaryType == canary.CanaryTypeMirroring && !ms.IsHaveHTTP() {
		return fmt.Errorf("镜像流量类型的灰度发布，仅支持端口协议为http、http2、grpc的服务")
	}

	//noHttp requestHeader check
	if cay.Policy != nil && cay.Policy.RequestStrategy != nil && (!ms.IsHaveHTTP() || ms.IsGrpcNoHttp()) && cay.CanaryType == canary.CanaryTypeCanary {
		return fmt.Errorf("金丝雀类型的灰度发布，按请求内容分配流量，仅支持端口协议为http、http2的服务")
	}
	return nil
}

type ParamsOptions struct {
	Name         string
	CanaryType   string
	MicroApp     string
	MicroService string
	ListOptions  metav1.ListOptions
}

func ListCanary(mgr multiCluster.Manager, ctx iris.Context) {

	appCtx := handler.ExtractAppContext(ctx)

	clusterName := appCtx.ClusterName
	namespace := appCtx.KubeNamespace
	microapp := ctx.Params().GetString("application")

	var canaryGVK = &schema.GroupVersionKind{
		Group:   common.ServiceMeshGroup,
		Version: common.ServiceMeshVersion,
		Kind:    common.CanaryKind,
	}

	canaryClient, err := mgr.DynamicClient(clusterName, canaryGVK)
	if err != nil {
		logger.Errorf("DynamicClient %+v, %s", *canaryGVK, err)

		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))
		return

	}

	microServiceClient, err := mgr.DynamicClient(clusterName, MicroServiceGVK)
	if err != nil {
		logger.Errorf("DynamicClient %+v, %s", *MicroServiceGVK, err)

		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))
		return

	}

	selectorlabel := make(map[string]string)
	selectorlabel[define.LabelTpaasApp] = strconv.FormatInt(appCtx.AppId, 10)
	if len(microapp) > 0 {

		selectorlabel[common.LabelMicroApplicationKey] = microapp
	}

	listOptions := canary.ParamsOptions{
		Name:         ctx.URLParam("name"),
		CanaryType:   ctx.URLParam("canarytype"),
		MicroApp:     ctx.URLParam("microapp"),
		MicroService: ctx.URLParam("microservice"),
		Status:       ctx.URLParam("status"),
		ListOptions: metav1.ListOptions{

			LabelSelector: labels.Set(selectorlabel).String(),
		},
	}

	list, err := canary.ListCanary(ctx.Request().Context(), canaryClient, microServiceClient, namespace, listOptions)
	if err != nil {

		logger.Errorf("ListCanary %s,%s, err: %s", microapp, namespace, err)

		handler.RespondWithDetailedError(ctx, customErrors.CustomClientErr("获取灰度列表失败！", err))
		return
	}

	handler.ResponseOk(ctx, list)

}

func DeleteCanary(mgr multiCluster.Manager, ctx iris.Context) {

	appCtx := handler.ExtractAppContext(ctx)

	clusterName := appCtx.ClusterName
	namespace := appCtx.KubeNamespace
	canaryName := ctx.Params().GetString("canary")

	canaryClient, err := mgr.DynamicClient(clusterName, canaryGVK)
	if err != nil {
		logger.Errorf("DynamicClient %+v, %s", *canaryGVK, err)
		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))
		return

	}

	err = canary.DeleteCanary(ctx.Request().Context(), canaryClient, namespace, canaryName)
	if err != nil {

		logger.Errorf("Delete Canary %s.%s failed err: %s", canaryName, namespace, err)

		handler.RespondWithDetailedError(ctx, customErrors.CustomClientErr("删除灰度失败！", err))
		return
	}

	handler.SendAudit(audit.ModuleCanary, audit.ActionDelete, canaryName, ctx)
	handler.ResponseOk(ctx, nil)

}

func GetCanaryDetail(mgr multiCluster.Manager, ctx iris.Context) {

	appCtx := handler.ExtractAppContext(ctx)

	clusterName := appCtx.ClusterName
	namespace := appCtx.KubeNamespace
	canaryName := ctx.Params().GetString("canary")

	k8sClient, err := mgr.Client(clusterName)
	if err != nil {

		logger.Errorf("k8s client %s,err %s", clusterName, err)

		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))

		return
	}

	canaryClient, err := mgr.DynamicClient(clusterName, canaryGVK)
	if err != nil {
		logger.Errorf("DynamicClient %+v, %s", *canaryGVK, err)

		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))
		return

	}

	detail, err := canary.GetCanaryDetail(ctx.Request().Context(), k8sClient, canaryClient, namespace, canaryName)
	if err != nil {

		logger.Errorf("GetCanaryPolicy %s.%s failed err: %s", canaryName, namespace, err)

		handler.RespondWithDetailedError(ctx, customErrors.CustomClientErr("获取灰度任务失败！", err))
		return
	}

	handler.ResponseOk(ctx, detail)

}

func GetCanaryPolicy(mgr multiCluster.Manager, ctx iris.Context) {

	appCtx := handler.ExtractAppContext(ctx)

	clusterName := appCtx.ClusterName
	namespace := appCtx.KubeNamespace
	canaryName := ctx.Params().GetString("canary")

	canaryClient, err := mgr.DynamicClient(clusterName, canaryGVK)
	if err != nil {
		logger.Errorf("DynamicClient %+v, %s", *canaryGVK, err)

		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))
		return

	}
	microServiceClient, err := mgr.DynamicClient(clusterName, MicroServiceGVK)
	if err != nil {
		logger.Errorf("DynamicClient %+v, %s", *MicroServiceGVK, err)

		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))
		return

	}

	policy, err := canary.GetCanaryPolicy(ctx.Request().Context(), canaryClient, microServiceClient, namespace, canaryName)
	if err != nil {

		logger.Errorf("GetCanaryPolicy %s.%s failed err: %s", canaryName, namespace, err)

		handler.RespondWithDetailedError(ctx, customErrors.CustomClientErr("获取灰度任务规则失败！", err))
		return
	}

	handler.ResponseOk(ctx, policy)

}

func UpdateCanaryPolicy(mgr multiCluster.Manager, ctx iris.Context) {

	policy := &canary.CanaryPolicy{}
	err := ctx.ReadJSON(policy)
	if err != nil {
		handler.RespondWithDetailedError(ctx, customErrors.BadParametersErr(err))

		return
	}

	if err = ValidePolicy(policy); err != nil {

		handler.RespondWithError(ctx, customErrors.StatusCodeHTTPRequestErrorCode, err)
		return

	}

	appCtx := handler.ExtractAppContext(ctx)

	clusterName := appCtx.ClusterName
	namespace := appCtx.KubeNamespace
	canaryName := ctx.Params().GetString("canary")

	if len(policy.RequestStrategy) == 0 && len(policy.WeightStrategy) == 0 {
		err := fmt.Errorf("canary %s.%s  policy field required .", canaryName, namespace)

		logger.Errorf("UpdateCanaryPolicy %s.%s failed err: %s", canaryName, namespace, err)

		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))
		return
	}

	canaryClient, err := mgr.DynamicClient(clusterName, canaryGVK)
	if err != nil {
		logger.Errorf("DynamicClient %+v, %s", *canaryGVK, err)

		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))
		return

	}

	err = canary.UpdateCanaryPolicy(ctx.Request().Context(), canaryClient, namespace, canaryName, *policy)
	if err != nil {

		logger.Errorf("GetCanaryPolicy %s.%s failed err: %s", canaryName, namespace, err)

		handler.ResponseErr(ctx, err)
		return
	}

	handler.SendAudit(audit.ModuleCanary, audit.ActionUpdate, canaryName, ctx)

	handler.ResponseOk(ctx, nil)

}

func ValidePolicy(policy *canary.CanaryPolicy) error {
	if policy == nil || (len(policy.WeightStrategy) == 0 && len(policy.RequestStrategy) == 0 && len(policy.TrafficMirroring) == 0) {
		err := fmt.Errorf("缺少灰度规则设置.")

		return err
	}

	if len(policy.RequestStrategy) > 0 {
		for _, requestHead := range policy.RequestStrategy {
			for _, cookie := range requestHead.HttpCookies {
				if len(cookie.MatchValue) == 0 || len(cookie.MatchType) == 0 {
					err := fmt.Errorf("缺少灰度规则设置.")

					return err

				}
			}

			for _, header := range requestHead.HttpHeader {
				if len(header.MatchValue) == 0 || len(header.MatchType) == 0 {

					err := fmt.Errorf("缺少灰度规则设置.")

					return err
				}

			}
		}
	}
	if len(policy.WeightStrategy) > 0 {
		var totalWeight int32 = 0
		for _, weightPolicy := range policy.WeightStrategy {
			totalWeight = totalWeight + weightPolicy.Weight
		}

		if totalWeight != 100 {
			err := fmt.Errorf("流量规则比重大于或者是不等于100.")

			return err
		}
	}

	return nil
}

func GetCanaryMetriceSummary(mgr multiCluster.Manager, ctx iris.Context) {

	appCtx := handler.ExtractAppContext(ctx)

	clusterName := appCtx.ClusterName
	namespace := appCtx.KubeNamespace
	canaryName := ctx.Params().GetString("canary")

	k8sClient, err := mgr.Client(clusterName)
	if err != nil {

		logger.Errorf("k8s client %s,err %s", clusterName, err)

		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))

		return
	}

	p8sClient, err := prometheus.NewP8sClient(mgr, clusterName)
	if err != nil {
		logger.Errorf("prometheus Newclient err %v", err)
		msg := fmt.Sprintf("prometheus connection failed : %s", err)
		handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, msg)
		return

	}

	canaryClient, err := mgr.DynamicClient(clusterName, canaryGVK)
	if err != nil {
		logger.Errorf("DynamicClient %+v, %s", *canaryGVK, err)

		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))
		return

	}
	microServiceClient, err := mgr.DynamicClient(clusterName, MicroServiceGVK)
	if err != nil {
		logger.Errorf("DynamicClient %+v, %s", *MicroServiceGVK, err)
		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))
		return

	}
	result, err := canary.GetCanaryMetricsSummary(ctx.Request().Context(), k8sClient, p8sClient, canaryClient, microServiceClient, clusterName, namespace, canaryName)
	if err != nil {

		logger.Errorf("GetCanaryPolicy %s.%s failed err: %s", canaryName, namespace, err)

		handler.ResponseErr(ctx, err)
		return
	}

	handler.ResponseOk(ctx, result)

}

func GetCanaryMetriceWeight(mgr multiCluster.Manager, ctx iris.Context) {

	appCtx := handler.ExtractAppContext(ctx)

	clusterName := appCtx.ClusterName
	namespace := appCtx.KubeNamespace
	canaryName := ctx.Params().GetString("canary")

	p8sClient, err := prometheus.NewP8sClient(mgr, clusterName)
	if err != nil {
		logger.Errorf("prometheus Newclient err %v", err)

		handler.RespondWithDetailedError(ctx, customErrors.CustomClientErr("Prometheus 连接失败", err))
		return

	}

	canaryClient, err := mgr.DynamicClient(clusterName, canaryGVK)
	if err != nil {
		logger.Errorf("DynamicClient %+v, %s", *canaryGVK, err)

		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))
		return

	}
	microServiceClient, err := mgr.DynamicClient(clusterName, MicroServiceGVK)
	if err != nil {
		logger.Errorf("DynamicClient %+v, %s", *MicroServiceGVK, err)
		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))
		return

	}
	result, err := canary.GetCanaryMetriceWeight(ctx.Request().Context(), p8sClient, canaryClient, microServiceClient, namespace, canaryName)
	if err != nil {

		logger.Errorf("GetCanaryPolicy %s.%s failed err: %s", canaryName, namespace, err)

		handler.RespondWithDetailedError(ctx, customErrors.CustomClientErr("获取灰度流量监控失败！", err))
		return
	}

	handler.ResponseOk(ctx, result)

}

func GetCanaryMetrics(mgr multiCluster.Manager, ctx iris.Context) {

	appCtx := handler.ExtractAppContext(ctx)

	clusterName := appCtx.ClusterName
	namespace := appCtx.KubeNamespace
	canaryName := ctx.Params().GetString("canary")

	p8sClient, err := prometheus.NewP8sClient(mgr, clusterName)
	if err != nil {
		logger.Errorf("prometheus Newclient err %v", err)
		msg := fmt.Sprintf("prometheus connection failed : %s", err)
		handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, msg)
		return

	}

	canaryClient, err := mgr.DynamicClient(clusterName, canaryGVK)
	if err != nil {
		logger.Errorf("DynamicClient %+v, %s", *canaryGVK, err)
		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))
		return

	}
	microServiceClient, err := mgr.DynamicClient(clusterName, MicroServiceGVK)
	if err != nil {
		logger.Errorf("DynamicClient %+v, %s", *MicroServiceGVK, err)
		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))
		return

	}

	startTime, err := strconv.Atoi(ctx.URLParam(PathParameterStartTime))
	if err != nil {

		logger.Error("Error for PathParameterStartTime connot be empty")
		handler.ResponseErr(ctx, err)
		return
	}

	endTime, err := strconv.Atoi(ctx.URLParam(PathParameterEndTime))
	if err != nil {
		logger.Error("Error for PathParameterEndTime connot be empty")
		handler.ResponseErr(ctx, err)
		return
	}

	endTime = common.ParseEndTime(endTime)

	// default step
	step := DefaultStep

	stepStr := ctx.URLParam(PathParameterStep)
	if len(stepStr) != 0 {
		step, err = strconv.Atoi(stepStr)
		if err != nil {
			logger.Errorf("prase step err : %v", err)
			step = DefaultStep
		}
	}
	quantiles := ctx.URLParam(QueryParemeterQuantiles)

	result, err := canary.GetCanaryMetriceDetails(ctx.Request().Context(), p8sClient, canaryClient, microServiceClient, namespace, canaryName, startTime, endTime, step, quantiles)
	if err != nil {

		logger.Errorf("GetCanaryPolicy %s.%s failed err: %s", canaryName, namespace, err)

		handler.RespondWithDetailedError(ctx, customErrors.CustomClientErr("获取灰度流量监控失败！", err))
		return
	}

	handler.ResponseOk(ctx, result)
}

func TakeOverAllTraffic(mgr multiCluster.Manager, ctx iris.Context) {

	appCtx := handler.ExtractAppContext(ctx)

	clusterName := appCtx.ClusterName
	namespace := appCtx.KubeNamespace
	canaryName := ctx.Params().GetString("canary")
	version := ctx.URLParam(PathParameterVersion)

	canaryClient, err := mgr.DynamicClient(clusterName, canaryGVK)
	if err != nil {
		logger.Errorf("DynamicClient %+v, %s", *canaryGVK, err)
		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))
		return

	}

	err = canary.TakeOverAllTraffic(ctx.Request().Context(), canaryClient, namespace, canaryName, version)
	if err != nil {

		logger.Errorf("GetCanaryPolicy %s.%s failed err: %s", canaryName, namespace, err)

		handler.ResponseErr(ctx, err)
		return
	}

	handler.SendAudit(audit.ModuleCanary, audit.ActionUpdate, canaryName, ctx)

	handler.ResponseOk(ctx, nil)

}

func GoOffline(mgr multiCluster.Manager, ctx iris.Context) {

	appCtx := handler.ExtractAppContext(ctx)

	clusterName := appCtx.ClusterName
	namespace := appCtx.KubeNamespace
	canaryName := ctx.Params().GetString("canary")
	version := ctx.URLParam(PathParameterVersion)

	canaryClient, err := mgr.DynamicClient(clusterName, canaryGVK)
	if err != nil {
		logger.Errorf("DynamicClient %+v, %s", *canaryGVK, err)

		handler.RespondWithDetailedError(ctx, customErrors.DynamicClientErr(err))
		return

	}

	err = canary.GoOffline(ctx.Request().Context(), canaryClient, namespace, canaryName, version)
	if err != nil {

		logger.Errorf("GetCanaryPolicy %s.%s failed err: %s", canaryName, namespace, err)

		handler.ResponseErr(ctx, err)
		return
	}

	handler.SendAudit(audit.ModuleCanary, audit.ActionGOOFFLINE, canaryName, ctx)

	handler.ResponseOk(ctx, nil)

}

func WorkloadValidation(mgr multiCluster.Manager, ctx iris.Context) {

	result := ValidResult{}

	workloadName := ctx.Params().GetString("name")

	if len(workloadName) > DNS1123LabelMaxLength {
		err := fmt.Errorf("the length of canary workload name %s is more than 63.", workloadName)

		logger.Error(err)
		result.Valid = false
		result.Message = fmt.Sprintf("the length of canary workload name %s is more than 63.", workloadName)
		handler.ResponseOk(ctx, &result)
		return

	}

	appCtx := handler.ExtractAppContext(ctx)

	clusterName := appCtx.ClusterName
	namespace := appCtx.KubeNamespace

	k8sClient, err := mgr.Client(clusterName)
	if err != nil {

		logger.Errorf("k8s client %s,err %s", clusterName, err)

		result.Valid = false
		result.Message = fmt.Sprintf("k8s client %s,err %s", clusterName, err)
		handler.ResponseOk(ctx, &result)

		return
	}

	_, err = k8sClient.AppsV1().Deployments(namespace).Get(ctx.Request().Context(), workloadName, metav1.GetOptions{})
	if err != nil && k8serror.IsNotFound(err) {
		result.Valid = true
		handler.ResponseOk(ctx, &result)
		return

	}

	result.Valid = false
	result.Message = fmt.Sprintf("workload %s is already exists", workloadName)
	handler.ResponseOk(ctx, &result)

}
func MicroServiceValidation(mgr multiCluster.Manager, ctx iris.Context) {

	serviceName := ctx.Params().GetString("name")

	valide := microServiceValidation(mgr, ctx, serviceName)

	handler.ResponseOk(ctx, valide)
}

func microServiceValidation(mgr multiCluster.Manager, ctx iris.Context, serviceName string) (result *ValidResult) {

	result = &ValidResult{}

	appCtx := handler.ExtractAppContext(ctx)

	clusterName := appCtx.ClusterName
	namespace := appCtx.KubeNamespace

	microapp := ctx.Params().GetString("application")

	selectorlabel := make(map[string]string)
	selectorlabel[common.LabelMicroServiceKey] = serviceName

	listOptions := metav1.ListOptions{
		LabelSelector: labels.Set(selectorlabel).String(),
	}

	// check canary
	canaryDC, err := mgr.DynamicClient(clusterName, &schema.GroupVersionKind{
		Group:   define.ServiceMeshGroup,
		Version: define.ServiceMeshVersion,
		Kind:    define.CanaryKind,
	})
	if err != nil {
		logger.Errorf("fetch canary %s err %s", serviceName, err)

	}

	if canaryDC != nil {
		unList, err := canaryDC.Namespace(namespace).List(ctx.Request().Context(), listOptions)
		if err != nil {
			logger.Errorf("fetch canary %s err %s", serviceName, err)
		}

		if unList != nil && len(unList.Items) > 0 {
			result.Valid = false
			result.Message = fmt.Sprintf("服务%s已有灰度发布任务，不可重复添加", serviceName)
			return
		}

	}

	// check microapp

	if _, err := dynamicFoundResource(mgr, &schema.GroupVersionKind{
		Group:   define.ServiceMeshGroup,
		Version: define.ServiceMeshVersion,
		Kind:    define.MicroApplicationKind,
	}, clusterName, namespace, microapp); err != nil {

		logger.Errorf("fetch microapplication %s err %s", microapp, err)
		result.Valid = false
		result.Message = fmt.Sprintf("微服务应用%s不存在", microapp)
		return
	}

	// check microservice
	microService, err := fetchMicroservice(mgr, ctx, serviceName)
	if err != nil {
		result.Valid = false
		result.Message = fmt.Sprintf("服务%s不存在", serviceName)
		return
	}

	// check svc

	k8sClient, err := mgr.Client(clusterName)
	if err != nil {

		logger.Errorf("k8s client %s,err %s", clusterName, err)

		result.Valid = false
		result.Message = fmt.Sprintf("服务%s不存在", serviceName)
		return
	}

	_, err = k8sClient.CoreV1().Services(namespace).Get(ctx.Request().Context(), serviceName, metav1.GetOptions{})
	if err != nil && k8serror.IsNotFound(err) {
		result.Valid = false
		result.Message = fmt.Sprintf("服务%s不存在", serviceName)
		return

	}

	// check workload
	deployments, err := k8sClient.AppsV1().Deployments(namespace).List(ctx.Request().Context(), listOptions)
	if err != nil {
		result.Valid = false
		result.Message = fmt.Sprintf("工作负载%s不存在", serviceName)
		return

	}

	if len(deployments.Items) == 0 {
		result.Valid = false
		result.Message = fmt.Sprintf("工作负载%s不存在", serviceName)
		return

	}

	for _, deploy := range deployments.Items {
		if *deploy.Spec.Replicas == 0 {
			result.Valid = false
			result.Message = fmt.Sprintf("工作负载%s不存在", serviceName)
			return
		}
	}

	if !microService.DeletionTimestamp.IsZero() || microService.Status.Phase != microV1beta1.MicroServicePhaseRunning {
		result.Valid = false
		result.Message = fmt.Sprintf("服务%s不存在", serviceName)
		return

	}

	if !reflect.ValueOf(microService.Spec.CanaryWorkload).IsZero() {
		result.Valid = false
		result.Message = fmt.Sprintf("服务%s已有灰度发布任务，不可重复添加", serviceName)
		return

	}

	result.Valid = true
	return

}

func dynamicFoundResource(mgr multiCluster.Manager, gvk *schema.GroupVersionKind, clusterName, namespace, name string) (*unstructured.Unstructured, error) {

	dc, err := mgr.DynamicClient(clusterName, gvk)
	if err != nil {
		logger.Errorf("k8s dynamic client %s,err %s", clusterName, err)
		return nil, err
	}

	resource, err := dc.Namespace(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("fetch resource %s err %s", name, err)
		return nil, err
	}
	return resource, nil

}

func fetchMicroservice(mgr multiCluster.Manager, ctx iris.Context, serviceName string) (*microV1beta1.MicroService, error) {

	appCtx := handler.ExtractAppContext(ctx)

	clusterName := appCtx.ClusterName
	namespace := appCtx.KubeNamespace

	microServiceClient, err := mgr.DynamicClient(clusterName, MicroServiceGVK)
	if err != nil {

		return nil, err

	}

	us, err := microServiceClient.Namespace(namespace).Get(ctx.Request().Context(), serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	microService := &microV1beta1.MicroService{}
	if err = common.JsonConvert(&us, microService); err != nil {
		return nil, err
	}

	return microService, nil
}

func AvailableWorkloadValidation(mgr multiCluster.Manager, ctx iris.Context) {

	serviceName := ctx.Params().GetString("service")
	workloadName := ctx.Params().GetString("name")

	valide := workloadValidation(mgr, ctx, workloadName, serviceName)

	handler.ResponseOk(ctx, valide)
}

func workloadValidation(mgr multiCluster.Manager, ctx iris.Context, workloadName, serviceName string) (result *ValidResult) {

	result = &ValidResult{}

	appCtx := handler.ExtractAppContext(ctx)

	clusterName := appCtx.ClusterName
	namespace := appCtx.KubeNamespace

	// check svc

	k8sClient, err := mgr.Client(clusterName)
	if err != nil {

		logger.Errorf("k8s client %s,err %s", clusterName, err)

		result.Valid = false
		result.Message = fmt.Sprintf("工作负载%s不存在", workloadName)
		return
	}

	svcs, err := k8sClient.CoreV1().Services(namespace).List(ctx.Request().Context(), metav1.ListOptions{})
	if err != nil && k8serror.IsNotFound(err) {
		result.Valid = false
		result.Message = fmt.Sprintf("工作负载%s不存在", workloadName)
		return

	}

	// check workload
	deploy, err := k8sClient.AppsV1().Deployments(namespace).Get(ctx.Request().Context(), workloadName, metav1.GetOptions{})
	if err != nil {
		result.Valid = false
		result.Message = fmt.Sprintf("工作负载%s不存在", workloadName)
		return

	}

	if deploy == nil {
		result.Valid = false
		result.Message = fmt.Sprintf("工作负载%s不存在", workloadName)
		return

	}

	if microType, ok := deploy.GetLabels()[define.LabelReleaseType]; ok && microType == define.ReleaseTypeMicroService {

		if microApp, ok := deploy.GetLabels()[define.LabelRelease]; ok {
			result.Valid = false
			result.Message = fmt.Sprintf("工作负载%s已经关联应用%s", workloadName, microApp)
			return
		}

	}

	for _, svc := range svcs.Items {
		if IsSelectorMatching(svc.Spec.Selector, deploy.Spec.Template.ObjectMeta.Labels) && svc.Name != serviceName {

			result.Valid = false
			result.Message = fmt.Sprintf("工作负载%s已经关联服务%s", workloadName, svc.Name)
			return
		}
	}

	// check canary
	if canary, ok := deploy.GetLabels()[common.LabelWorkloadCanary]; ok {
		result.Valid = false
		result.Message = fmt.Sprintf("工作负载%s已经关联到灰度发布%s", workloadName, canary)
		return

	}

	result.Valid = true
	return

}

func IsSelectorMatching(labelSelector map[string]string, testedObjectLabels map[string]string) bool {
	// If service has no selectors, then assume it targets different resource.
	if len(labelSelector) == 0 {
		return false
	}
	for label, value := range labelSelector {
		if rsValue, ok := testedObjectLabels[label]; !ok || rsValue != value {
			return false
		}
	}
	return true
}
