package installation

import (
	"context"
	"fmt"

	k8serror "k8s.io/apimachinery/pkg/api/errors"

	customErrors "github.com/huhenry/hej/pkg/errors"
	"github.com/kataras/iris/v12"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/huhenry/hej/pkg/common"
	"github.com/huhenry/hej/pkg/handler"
	"github.com/huhenry/hej/pkg/installation/operator"
	"github.com/huhenry/hej/pkg/log"
	"github.com/huhenry/hej/pkg/multiCluster"
	istiov1alpha1 "istio.io/istio/operator/pkg/apis/istio/v1alpha1"
)

var logger = log.RegisterScope("installation.handler")

const (
	ServiceMeshGroup   = "microservices.troila.com"
	ServiceMeshVersion = "v1beta1"
	ServiceMeshKind    = "ServiceMesh"

	DefaultInstallNamespace = "istio-system"
	DefaultProfile          = "default"

	JaegerPort = "5066"
)

var ServiceMeshGVK = &schema.GroupVersionKind{
	Group:   ServiceMeshGroup,
	Version: ServiceMeshVersion,
	Kind:    ServiceMeshKind,
}

func Install(mgr multiCluster.Manager, ctx iris.Context) {
	istioInstall := &operator.IstioInstallOptions{}
	ctx.ReadJSON(istioInstall)
	clusterName := ctx.Params().Get("cluster")
	factory := operator.NewOperatorManagerFactory()
	//默认开启zk
	istioInstall.Zookeeper = true
	manager, err := factory(clusterName, mgr)
	if err != nil {
		logger.Errorf("install failed err: %s", err)

		handler.RespondWithDetailedError(ctx, customErrors.CustomClientErr("Istio安装部署失败！", err))

		return

	}
	err = manager.Install(istioInstall)
	if err != nil {

		manager.UnInstall()

		logger.Errorf("installation failed %s", err)

		handler.RespondWithDetailedError(ctx, customErrors.CustomClientErr("Istio安装部署失败！", err))

		return
	}

	//handler.SendAudit(audit.ModuleIstio, audit.ActionInstall, clusterName, ctx)

	handler.ResponseOk(ctx, "{Success:true}")

}

func Uninstall(mgr multiCluster.Manager, ctx iris.Context) {

	clusterName := ctx.Params().Get("cluster")

	factory := operator.NewOperatorManagerFactory()

	manager, err := factory(clusterName, mgr)
	if err != nil {
		logger.Errorf("UnInstall failed err: %s", err)

		handler.RespondWithDetailedError(ctx, customErrors.CustomClientErr("Istio卸载失败！", err))

		return

	}

	err = manager.UnInstall()
	if err != nil {
		logger.Errorf("UnInstall failed err: %s", err)

	}
	//handler.SendAudit(audit.ModuleIstio, audit.ActionUnInstall, clusterName, ctx)
	handler.ResponseOk(ctx, "{Success:true}")

}

type ServiceMesh struct {
	Spec ServiceMeshSpec `json:"spec"`
}

type ServiceMeshSpec struct {
	// JeagerHost is the host of Jaeger query service.
	JaegerHost string `json:"jaegerHost,omitempty"`
	// ApiServerHost is the host  of k8s APIService.
	ApiServerHost string `json:"apiserverHost,omitempty"`

	WireAddress string `json:"wireAddress,omitempty"`

	ZookeeperAddress string `json:"zookeeperAddress,omitempty"`
}
type ClusterStatus struct {
	Status        string `json:"status"`
	Error         string `json:"error_message,omitempty"`
	Host          string `json:"host,omitempty"`
	JaegerURL     string `json:"jaegerHost,omitempty"`
	PrometheusURL string `json:"prometheusHost,omitempty"`
	EurekaHost    string `json:"eurekaHost,omitempty"`
	ZookeeperHost string `json:"zookeeperHost,omitempty"`
}

type EgressgatewayStatus struct {
	Status string `json:"status"`
	Enable bool   `json:"enable"`
	Error  string `json:"error"`
}

func Status(mgr multiCluster.Manager, ctx iris.Context) {

	clusterName := ctx.Params().Get("cluster")
	cluster := ClusterStatus{
		Status: "success",
	}

	dynamicClient, err := mgr.DynamicClient(clusterName, &istiov1alpha1.IstioOperatorGVK)
	if err != nil {
		logger.Errorf("DynamicClient %s", err)
		cluster.Status = "failure"
		cluster.Error = fmt.Sprintf("%s", err)

		handler.ResponseOk(ctx, cluster)
		return

	}

	_, err = dynamicClient.Namespace("istio-system").Get(context.TODO(), "istiocontrolplane-default", metav1.GetOptions{})
	if err != nil {
		logger.Errorf("istiocontrolplane-default %s", err)
		cluster.Status = "failure"
		cluster.Error = fmt.Sprintf("%s", err)

		handler.ResponseOk(ctx, cluster)
		return
	}

	meshClient, err := mgr.DynamicClient(clusterName, ServiceMeshGVK)
	if err != nil {
		logger.Errorf("meshclient err %s", err)
		cluster.Status = "failure"
		cluster.Error = fmt.Sprintf("%s", err)

		handler.ResponseOk(ctx, cluster)
		return
	}

	meshus, err := meshClient.Namespace("istio-system").Get(context.TODO(), "mesh", metav1.GetOptions{})
	if err != nil {
		logger.Errorf("fetch mesh err %s", err)
		cluster.Status = "failure"
		cluster.Error = fmt.Sprintf("%s", err)

		handler.ResponseOk(ctx, cluster)
		return
	}

	mesh := &ServiceMesh{}
	if err := common.JsonConvert(meshus, mesh); err != nil {
		logger.Errorf("fetch mesh err %s", err)
		cluster.Status = "failure"
		cluster.Error = fmt.Sprintf("%s", err)

		handler.ResponseOk(ctx, cluster)
		return
	}

	clusterhost := mesh.Spec.ApiServerHost

	jaegerhost := mesh.Spec.JaegerHost

	wireAddress := mesh.Spec.WireAddress
	zookeeperAddress := mesh.Spec.ZookeeperAddress
	cluster = ClusterStatus{
		Status:        "success",
		Host:          clusterhost,
		JaegerURL:     jaegerhost,
		EurekaHost:    wireAddress,
		ZookeeperHost: zookeeperAddress,
	}

	handler.ResponseOk(ctx, cluster)

}

func EgressStatus(mgr multiCluster.Manager, ctx iris.Context) {
	clusterName := ctx.Params().Get("cluster")
	istioOperatorClient, err := mgr.DynamicClient(clusterName, &istiov1alpha1.IstioOperatorGVK)
	egress := &EgressgatewayStatus{}
	if err != nil {
		logger.Errorf("istioOperatorClient %s", err)
		egress.Status = "failure"
		egress.Error = fmt.Sprintf("%s", err)
		handler.ResponseOk(ctx, egress)
		return
	}

	un, err := istioOperatorClient.Namespace("istio-system").Get(context.TODO(), "istiocontrolplane-default", metav1.GetOptions{})
	if err != nil {
		logger.Errorf("istiocontrolplane-default %s", err)
		egress.Status = "failure"
		egress.Error = fmt.Sprintf("%s", err)

		handler.ResponseOk(ctx, egress)
		return
	}
	/*	istioOperator := &istiov1alpha1.IstioOperator{}
		err = dynamic.FromUnstructured(un, istioOperator)
		if err != nil {
			logger.Errorf("istioOperator convert %s", err)
			egress.Status = "failure"
			egress.Error = fmt.Sprintf("%s", err)

			handler.ResponseOk(ctx, egress)
			return
		}
				if istioOperator.Spec.Components != nil &&istioOperator.Spec.Components.EgressGateways!=nil&&len(istioOperator.Spec.Components.EgressGateways)>0{
			egress.Enable = istioOperator.Spec.Components.EgressGateways[0].Enabled.Value
		}
	*/
	spec := un.Object["spec"]
	if spec == nil {
		err = fmt.Errorf("istiocontrolplane-default spec is null")
		logger.Errorf(err.Error())
		egress.Status = "failure"
		egress.Error = fmt.Sprintf("%s", err)
		handler.ResponseOk(ctx, egress)
		return
	}
	specmap, ok := spec.(map[string]interface{})
	if !ok {
		err = fmt.Errorf("istiocontrolplane-default spec not is map")
		logger.Errorf(err.Error())
		egress.Status = "failure"
		egress.Error = fmt.Sprintf("%s", err)
		handler.ResponseOk(ctx, egress)
		return
	}
	components := specmap["components"]
	componentsmap, ok := components.(map[string]interface{})
	if !ok {
		err = fmt.Errorf("istiocontrolplane-default spec components not is map")
		logger.Errorf(err.Error())
		egress.Status = "failure"
		egress.Error = fmt.Sprintf("%s", err)
		handler.ResponseOk(ctx, egress)
		return
	}
	egressGateways := componentsmap["egressGateways"]
	egressGatewayslist, ok := egressGateways.([]interface{})
	if !ok {
		err = fmt.Errorf("istiocontrolplane-default spec components egressGateways not is slice")
		logger.Errorf(err.Error())
		egress.Status = "failure"
		egress.Error = fmt.Sprintf("%s", err)
		handler.ResponseOk(ctx, egress)
		return
	}
	if egressGatewayslist == nil || len(egressGatewayslist) == 0 {
		err = fmt.Errorf("istiocontrolplane-default spec components egressGateways is null")
		logger.Errorf(err.Error())
		egress.Status = "failure"
		egress.Error = fmt.Sprintf("%s", err)
		handler.ResponseOk(ctx, egress)
		return
	}
	egresses := egressGatewayslist[0]
	egressesmap, ok := egresses.(map[string]interface{})
	if !ok {
		err = fmt.Errorf("istiocontrolplane-default spec components egressGateways egress not is map")
		logger.Errorf(err.Error())
		egress.Status = "failure"
		egress.Error = fmt.Sprintf("%s", err)
		handler.ResponseOk(ctx, egress)
		return
	}

	if v, ok := egressesmap["enabled"]; !ok {
		err = fmt.Errorf("istiocontrolplane-default spec components egressGateways egress enabled is null")
		logger.Errorf(err.Error())
		egress.Status = "failure"
		egress.Error = fmt.Sprintf("%s", err)
		handler.ResponseOk(ctx, egress)
		return
	} else {
		egress.Enable = v.(bool)
	}

	k8sclient, err := mgr.Client(clusterName)
	if err != nil {
		logger.Errorf("k8sclient %s", err)
		egress.Status = "failure"
		egress.Error = fmt.Sprintf("%s", err)
		handler.ResponseOk(ctx, egress)
		return
	}
	_, err = k8sclient.AppsV1().Deployments("istio-system").Get(context.TODO(), "istio-egressgateway", metav1.GetOptions{})
	if err != nil {
		if k8serror.IsNotFound(err) {
			egress.Status = "notFound"
		} else {
			egress.Status = "failure"
		}
		logger.Errorf("istio-egressGateway workload %s", err)
		egress.Error = fmt.Sprintf("%s", err)
		handler.ResponseOk(ctx, egress)
		return
	}
	egress.Status = "running"
	handler.ResponseOk(ctx, egress)
	return
}

func EgressEnable(mgr multiCluster.Manager, ctx iris.Context) {
	clusterName := ctx.Params().Get("cluster")

	factory := operator.NewOperatorManagerFactory()
	manager, err := factory(clusterName, mgr)
	if err != nil {
		logger.Errorf("EnableEgress failed err: %s", err)
		handler.RespondWithDetailedError(ctx, customErrors.CustomClientErr("外部服务开启失败！", err))

		return

	}
	operation := ctx.Params().Get("operation")
	enableEgress := false
	if operation == "enable" {
		enableEgress = true
	} else if operation == "disable" {
		enableEgress = false
	} else {
		logger.Errorf("EnableEgress failed err: %s", "params is invalid")
		err := fmt.Errorf("EnableEgress failed : operation %s is invalid", operation)
		handler.RespondWithDetailedError(ctx, customErrors.CustomClientErr("外部服务开启失败！", err))

		return
	}

	if err := manager.EnableEgress(enableEgress); err != nil {
		logger.Errorf("EnableEgress failed err: %s", err)
		err := fmt.Errorf("EnableEgress failed : %s", DefaultInstallNamespace, err)
		handler.RespondWithDetailedError(ctx, customErrors.CustomClientErr("外部服务开启失败！", err))

		return
	}
	/*
		if enableEgress {
			handler.SendAudit(audit.ModuleIstio+"-"+audit.ModuleServiceEntry, audit.ActionInstall, clusterName, ctx)
		} else {
			handler.SendAudit(audit.ModuleIstio+"-"+audit.ModuleServiceEntry, audit.ActionUnInstall, clusterName, ctx)
		}*/
	handler.ResponseOk(ctx, nil)
	return
}
