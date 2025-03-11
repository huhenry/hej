package microapp

import (
	"reflect"
	"sort"

	"github.com/huhenry/hej/pkg/microapp/resource"
	corev1 "k8s.io/api/core/v1"

	"github.com/huhenry/hej/pkg/common/app"
	"github.com/huhenry/hej/pkg/common/page"
	"github.com/huhenry/hej/pkg/define"
	"github.com/huhenry/hej/pkg/handler"
	micro "github.com/huhenry/hej/pkg/microapp"
	microappcommon "github.com/huhenry/hej/pkg/microapp/common"
	"github.com/huhenry/hej/pkg/microapp/v1beta1"
	"github.com/kataras/iris/v12"
)

const (
	ObjectStatusProgressing string = "Progressing"
	ObjectStatusRunning     string = "Running"
	ObjectStatusFailed      string = "Failed"
	ObjectStatusNotExisted  string = "NotExisted"
)

func ListWorkloadPods(ctx iris.Context) {
	appCtx := handler.ExtractAppContext(ctx)
	deploymentName := ctx.Params().GetString("workload")

	pods, err := micro.Resource().GetDeploymentPods(ctx.Request().Context(), appCtx.ClusterName, appCtx.KubeNamespace, deploymentName)
	//paramQuery := handler.ExtractQueryParam(ctx)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}

	handler.ResponseOk(ctx, pods)

}

func ListMicroService(ctx iris.Context) {
	application := ctx.Params().GetString("application")
	appCtx := handler.ExtractAppContext(ctx)
	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}
	paramQuery := handler.ExtractQueryParam(ctx)

	microservices, err := micro.MicroService().List(resource, application, true)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}
	stats, err := micro.Resource().GetStats(appCtx.ClusterName, appCtx.AppId)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}
	list := entity2List(microservices, application, stats)

	if err != nil {
		handler.ResponseErr(ctx, err)
	} else {
		sort.Slice(list, func(i, j int) bool {
			r := list[i].CreateTimeSec > list[j].CreateTimeSec
			if paramQuery.Sortby == define.SortByCreateTime {
				r = !r
			}

			return r
		})
		data := make([]interface{}, 0)
		for i := range list {
			data = append(data, list[i])
		}

		handler.ResponseOk(ctx, page.PageInfo(data, paramQuery))
	}
}

func entity2List(source []v1beta1.MicroServiceEntity, application string, stats *resource.ServiceWorkloadStats) []*MicroServiceListItem {
	list := make([]*MicroServiceListItem, 0, len(source))
	for i := range source {

		item := &MicroServiceListItem{}
		item.Name = source[i].Name
		item.Application = application
		item.MicroServiceStatus = ParseMicroserviceStatus(source[i], stats)
		item.CreationInfo = source[i].CreationInfo
		item.AppResources = source[i].AppResources
		item.Workload = newProCrossNamespaceOR(source[i].Workload, stats)
		item.CanaryWorkload = newProCrossNamespaceOR(source[i].CanaryWorkload, stats)
		item.Version = source[i].Version
		item.CanaryVersion = source[i].CanaryVersion
		ports, _ := microappcommon.CovertPorts(source[i].Ports)
		item.Ports = ports

		list = append(list, item)
	}

	return list
}

func ParseMicroserviceStatus(ms v1beta1.MicroServiceEntity, stats *resource.ServiceWorkloadStats) v1beta1.MicroServiceStatus {

	if !reflect.ValueOf(ms.MicroServiceStatus).IsZero() {
		return ParseStatus(ms.MicroServiceStatus)
	}

	status := v1beta1.MicroServiceStatus{
		Phase: v1beta1.MicroServicePhaseRunning,
	}

	//协议全是UDP 返回异常
	_, allUDP := microappcommon.CovertPorts(ms.Ports)

	if allUDP {
		status.Phase = v1beta1.MicroServicePhaseProtocolUDP
		status.Conditions = []v1beta1.MicroServiceCondition{
			v1beta1.MicroServiceCondition{
				Message: v1beta1.ConditionPhaseTrans[v1beta1.MicroServicePhaseProtocolUDP],
			},
		}
		return status
	}

	var desiredReplicas, availableReplicas int32 = 0, 0

	if workload, ok := stats.GetDeployment(ms.Workload.Name); ok {
		if workload.Config != nil {
			desiredReplicas = *workload.Config.Spec.Replicas
			availableReplicas = workload.Config.Status.AvailableReplicas

			status.DesiredReplicas = desiredReplicas
			status.ReadyReplicas = availableReplicas
			if desiredReplicas == 0 || availableReplicas == 0 {
				status.Phase = v1beta1.MicroServicePhaseWorkloadNoReplicas
				status.Conditions = []v1beta1.MicroServiceCondition{
					v1beta1.MicroServiceCondition{
						Message: v1beta1.ConditionPhaseTrans[v1beta1.MicroServicePhaseWorkloadNoReplicas],
					},
				}
				return status
			}

			if desiredReplicas != availableReplicas {

				status.Phase = v1beta1.MicroServicePhaseProgressing
				return status
			}
		}

	}
	if workload, ok := stats.GetDeployment(ms.CanaryWorkload.Name); ok {
		if workload.Config != nil {
			desiredReplicas = desiredReplicas + *workload.Config.Spec.Replicas
			availableReplicas = availableReplicas + workload.Config.Status.AvailableReplicas

			status.DesiredReplicas = desiredReplicas
			status.ReadyReplicas = availableReplicas
			if desiredReplicas == 0 || availableReplicas == 0 {
				status.Phase = v1beta1.MicroServicePhaseWorkloadNoReplicas
				status.Conditions = []v1beta1.MicroServiceCondition{
					v1beta1.MicroServiceCondition{
						Message: v1beta1.ConditionPhaseTrans[v1beta1.MicroServicePhaseWorkloadNoReplicas],
					},
				}
				return status
			}

			if desiredReplicas != availableReplicas {

				status.Phase = v1beta1.MicroServicePhaseProgressing
				return status
			}
		}
	}

	return status

}

func ParseStatus(source v1beta1.MicroServiceStatus) v1beta1.MicroServiceStatus {

	dest := source
	destConditons := make([]v1beta1.MicroServiceCondition, 0)

	for _, sourceCondition := range source.Conditions {
		destConditons = append(destConditons, v1beta1.MicroServiceCondition{
			Reason:  sourceCondition.Reason,
			Message: v1beta1.TransMessage(sourceCondition),
		})
	}
	dest.Conditions = destConditons
	return dest

}

type ProCrossNamespaceObjectReference struct {
	APIVersion        string `json:"apiVersion,omitempty"`
	Kind              string `json:"kind,omitempty"`
	Name              string `json:"name"`
	Namespace         string `json:"namespace,omitempty"`
	DesiredReplicas   int32  `json:"desiredReplicas,omitempty"`
	AvailableReplicas int32  `json:"availableReplicas,omitempty"`
	Status            string `json:"status,omitempty"`
}

func newProCrossNamespaceOR(source v1beta1.CrossNamespaceObjectReference, stats *resource.ServiceWorkloadStats) *ProCrossNamespaceObjectReference {

	dst := &ProCrossNamespaceObjectReference{
		APIVersion: source.APIVersion,
		Kind:       source.Kind,
		Name:       source.Name,
		Namespace:  source.Namespace,
	}
	if len(source.Name) == 0 {
		return dst
	}

	if deploy, ok := stats.GetDeployment(dst.Name); ok {
		if deploy.Config != nil {
			dst.DesiredReplicas = *deploy.Config.Spec.Replicas
			dst.AvailableReplicas = deploy.AvailableReplicas
		}
	}
	if len(source.Status) > 0 {
		dst.Status = source.Status
	} else {
		if dst.DesiredReplicas == 0 {
			dst.Status = ObjectStatusFailed
		} else if dst.DesiredReplicas == dst.AvailableReplicas {
			dst.Status = ObjectStatusRunning
		} else {
			dst.Status = ObjectStatusProgressing
		}
	}

	return dst
}

type MicroServiceSpecItem struct {
	Version        string                            `json:"version,omitempty"`
	Workload       *ProCrossNamespaceObjectReference `json:"workload,omitempty"`
	ServiceName    string                            `json:"service,omitempty"`
	Ports          []corev1.ServicePort              `json:"ports,omitempty"`
	CanaryVersion  string                            `json:"canaryVersion,omitempty"`
	CanaryWorkload *ProCrossNamespaceObjectReference `json:"canaryWorkload,omitempty"`
}

type MicroServiceListItem struct {
	Name        string `json:"name"`
	Application string `json:"application, omitempty"`
	MicroServiceSpecItem
	v1beta1.MicroServiceStatus
	app.AppResources
	app.CreationInfo
}
