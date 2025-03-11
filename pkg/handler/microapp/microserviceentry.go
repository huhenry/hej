package microapp

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/huhenry/hej/pkg/common"
	"github.com/huhenry/hej/pkg/common/app"
	"github.com/huhenry/hej/pkg/common/page"
	"github.com/huhenry/hej/pkg/define"
	microapierrors "github.com/huhenry/hej/pkg/errors"
	"github.com/huhenry/hej/pkg/handler"
	"github.com/huhenry/hej/pkg/handler/audit"
	micro "github.com/huhenry/hej/pkg/microapp"
	"github.com/huhenry/hej/pkg/microapp/v1beta1"
	"github.com/huhenry/hej/pkg/multiCluster"
	"github.com/kataras/iris/v12"
	"istio.io/istio/pkg/config/validation"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	infratroilacomv1beta1 "paas.gitlab.com/paas/cluster_config/api/v1beta1"
)

const (
	LocationExternal string = "MESH_EXTERNAL"
	LocationInternal string = "MESH_INTERNAL"

	ResolutionNone   string = "NONE"
	ResolutionStatic string = "STATIC"
	ResolutionDNS    string = "DNS"
)

var (
	supportedProtocol = map[string]bool{
		"HTTP":  true,
		"HTTPS": true,
		"TCP":   true,
		"GRPC":  true,
	}
	supportedResolution = map[string]bool{
		"NONE":   true,
		"STATIC": true,
		"DNS":    true,
	}
)

type MicroServiceEntryCreation struct {
	ServiceName string   `json:"serviceName"`
	Hosts       []string `json:"hosts"`
	Resolution  string   `json:"resolution"`
	Ports       []Port   `json:"ports"`
	ExportTo    []string `json:"exportTo"`
	Endpoints   []string `json:"endpoints"`
	Description string   `json:"description"`
}

type MicroServiceEntryItem struct {
	Name        string `json:"name"`
	Application string `json:"application,omitempty"`
	MicroServiceEntryCreation
	app.AppResources
	app.CreationInfo
}
type Port struct {
	// A valid non-negative integer port number.
	Number int32 ` json:"number,omitempty"`
	// MUST BE one of HTTP|HTTPS|TCP|GRPC
	Protocol string `json:"protocol,omitempty"`
	Name     string `json:"name,omitempty"`
}

func CreateMicroServiceEntry(mgr multiCluster.Manager, ctx iris.Context) {
	creation := &MicroServiceEntryCreation{}

	if creation.ExportTo == nil || len(creation.ExportTo) == 0 {
		creation.ExportTo = []string{"*"}
	}
	err := ctx.ReadJSON(creation)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}

	if err = validateMicroServiceEntryCreation(mgr, false, ctx, creation); err != nil {
		handler.ResponseErr(ctx, err)
		return
	}
	ms := buildMicroServiceEntryEntity(ctx, creation)
	err = micro.MicroServiceEntry().Create(ms)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	} else {
		for _, host := range ms.ServiceEntry.Hosts {
			createDomainValidation(mgr, ctx, host)
		}
		handler.SendAudit(audit.ModuleMicroApplication, audit.ActionCreate+audit.ModuleServiceEntry, ms.Application+"/"+ms.Name, ctx)
		handler.ResponseOk(ctx, nil)
	}
}

func UpdateMicroServiceEntry(mgr multiCluster.Manager, ctx iris.Context) {
	creation := &MicroServiceEntryCreation{}

	appCtx := handler.ExtractAppContext(ctx)
	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}
	err := ctx.ReadJSON(creation)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}

	if err = validateMicroServiceEntryCreation(mgr, true, ctx, creation); err != nil {
		handler.ResponseErr(ctx, err)
		return
	}
	ms := buildMicroServiceEntryEntity(ctx, creation)
	err = micro.MicroServiceEntry().Update(resource, ms)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	} else {
		handler.SendAudit(audit.ModuleMicroApplication, audit.ActionPut+audit.ModuleServiceEntry, ms.Application+"/"+ms.Name, ctx)
		handler.ResponseOk(ctx, nil)
	}
}

func validateMicroServiceEntryCreation(mgr multiCluster.Manager, isUpdate bool, ctx iris.Context, creation *MicroServiceEntryCreation) error {
	if creation == nil {
		return microapierrors.BadRequest("服务数据不能为空")
	}
	if creation.ServiceName == "" {
		return microapierrors.BadRequest("服务名不能为空")
	}
	if len(creation.Hosts) == 0 {
		return microapierrors.BadRequest("主机地址/域名不能为空")
	}
	if creation.Resolution == "" {
		return microapierrors.BadRequest("服务发现模式不能为空")
	}
	if len(creation.Ports) == 0 {
		return microapierrors.BadRequest("至少需要一组可用端口")
	}
	if creation.Resolution == ResolutionStatic && len(creation.Endpoints) == 0 {
		return microapierrors.BadRequest(fmt.Sprintf("服务发现模式为%s时，端点不能为空", ResolutionStatic))
	}
	if !supportedResolution[creation.Resolution] {
		return microapierrors.BadRequest(fmt.Sprintf("服务发现模式%s无效", creation.Resolution))
	}
	invalidIps := ""
	ipRepeatCheck := make(map[string]bool)
	for _, endpoint := range creation.Endpoints {
		if ip := net.ParseIP(endpoint); ip == nil {
			invalidIps = endpoint + ","
		}
		if _, ok := ipRepeatCheck[endpoint]; ok {
			return microapierrors.BadRequest(fmt.Sprintf("ip地址%s重复", endpoint))
		} else {
			ipRepeatCheck[endpoint] = true
		}
	}
	if invalidIps != "" {
		invalidIps = invalidIps[0 : len(invalidIps)-2]
		return microapierrors.BadRequest(fmt.Sprintf("端点：%s是无效的", invalidIps))
	}
	for _, port := range creation.Ports {
		if !supportProtocol(port.Protocol) {
			return microapierrors.BadRequest(fmt.Sprintf("协议：%s暂不支持", port.Protocol))
		}
		if portIsvalid(port.Number) {
			return microapierrors.BadRequest(fmt.Sprintf("端口号：%d超出范围", port.Number))
		}
	}

	//更新是无需校验host
	if isUpdate {
		return nil
	}
	for _, h := range creation.Hosts {
		if err := validation.ValidateFQDN(h); err != nil {
			return microapierrors.BadRequest(fmt.Sprintf("主机地址/域名：%s,格式错误", h))
		}
		/*if creation.Resolution != ResolutionStatic {*/
		if err := DomainValidationServiceEntry(mgr, ctx, h); err != nil {
			return microapierrors.BadRequest(err.Error())
		}
		/*}*/
	}

	return nil
}

func portIsvalid(number int32) bool {
	if number <= 0 || number > 65535 {
		return true
	}
	return false
}

func supportProtocol(protocol string) bool {
	return supportedProtocol[protocol]
}

func buildMicroServiceEntryEntity(ctx iris.Context, creation *MicroServiceEntryCreation) *v1beta1.MicroServiceEntity {
	ms := &v1beta1.MicroServiceEntity{}
	appCtx := handler.ExtractAppContext(ctx)
	userCtx := handler.ExtractUserContext(ctx)
	ms.AppResources = app.AppResources{
		KubeNamespace: appCtx.KubeNamespace,
		Cluster:       appCtx.ClusterName,
		AppId:         appCtx.AppId,
		NamespaceId:   appCtx.NamespaceId,
	}
	ms.Creator = userCtx.Name
	ms.Name = creation.ServiceName
	ms.Application = ctx.Params().GetString("application")
	ms.ServiceName = creation.ServiceName
	genMicroServiceEntry(creation, ms)
	return ms
}

func genMicroServiceEntry(creation *MicroServiceEntryCreation, ms *v1beta1.MicroServiceEntity) {
	ports := make([]corev1.ServicePort, 0, len(creation.Ports))
	if len(creation.Ports) > 0 {
		for _, p := range creation.Ports {
			appProtocol := p.Protocol
			port := corev1.ServicePort{}
			port.Protocol = corev1.ProtocolTCP
			port.Name = fmt.Sprintf("%s-%d", p.Protocol, p.Number)
			port.Port = p.Number
			port.TargetPort = intstr.FromString(strconv.FormatInt(int64(p.Number), 10))
			port.AppProtocol = &appProtocol
			ports = append(ports, port)
		}
	}
	ms.Ports = ports

	serviceEntry := &v1beta1.ServiceEntry{
		Hosts:       creation.Hosts,
		Resolution:  creation.Resolution,
		Location:    LocationExternal,
		ExportTo:    creation.ExportTo,
		Endpoints:   creation.Endpoints,
		Description: creation.Description,
	}
	//ms.AppProtocol = &ms.Ports[0].Protocol
	ms.ServiceEntry = serviceEntry
}

func ListMicroServiceEntry(ctx iris.Context) {
	application := ctx.Params().GetString("application")
	appCtx := handler.ExtractAppContext(ctx)
	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}
	paramQuery := handler.ExtractQueryParam(ctx)

	microservices, err := micro.MicroServiceEntry().List(resource, application)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}
	keyName := strings.ToLower(paramQuery.Name)
	list := serviceEntryEntity2List(microservices, application, keyName)

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

func GetMicroServiceEntry(ctx iris.Context) {
	application := ctx.Params().GetString("application")
	serviceName := ctx.Params().GetString("name")
	appCtx := handler.ExtractAppContext(ctx)
	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}

	microservice, err := micro.MicroServiceEntry().Get(resource, serviceName, application)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}
	data := convertServiceEntry(microservice, application)

	handler.ResponseOk(ctx, data)

}

func convertServiceEntry(source *v1beta1.MicroServiceEntity, application string) *MicroServiceEntryItem {
	item := &MicroServiceEntryItem{}
	item.ServiceName = source.ServiceName
	item.Resolution = source.ServiceEntry.Resolution
	item.Hosts = source.ServiceEntry.Hosts
	ports := make([]Port, 0, len(source.Ports))
	if len(source.Ports) > 0 {
		for _, p := range source.Ports {
			port := Port{
				Number:   p.Port,
				Name:     p.Name,
				Protocol: *p.AppProtocol,
			}
			ports = append(ports, port)
		}

	}
	item.Ports = ports
	item.Endpoints = source.ServiceEntry.Endpoints
	item.Name = source.Name
	item.ExportTo = source.ServiceEntry.ExportTo
	item.Application = application
	item.CreationInfo = source.CreationInfo
	item.AppResources = source.AppResources
	item.Description = source.ServiceEntry.Description
	return item
}
func serviceEntryEntity2List(source []v1beta1.MicroServiceEntity, application, name string) []*MicroServiceEntryItem {

	list := make([]*MicroServiceEntryItem, 0, len(source))
	for i := range source {
		item := convertServiceEntry(&source[i], application)
		if name != "" {
			if strings.Contains(item.Name, name) {
				list = append(list, item)
			}
		} else {
			list = append(list, item)
		}

	}

	return list
}

func DeleteMicroServiceEntry(mgr multiCluster.Manager, ctx iris.Context) {
	name := ctx.Params().GetString("name")
	application := ctx.Params().GetString("application")
	keys := strings.Split(name, ",")
	appCtx := handler.ExtractAppContext(ctx)
	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}

	if keys == nil || len(keys) == 0 {
		err := fmt.Errorf("name is null")
		handler.ResponseErr(ctx, err)
		return
	}
	for _, k := range keys {
		ms, err := micro.MicroServiceEntry().Get(resource, k, application)
		if err != nil {
			handler.ResponseErr(ctx, err)
			return
		}

		if err == nil && ms.ServiceEntry != nil && ms.ServiceEntry.Hosts != nil && len(ms.ServiceEntry.Hosts) > 0 {
			RecoveryDomainValidation(mgr, ctx, ms.ServiceEntry.Hosts[0])
		}
		err = micro.MicroServiceEntry().Delete(resource, k)
		if err != nil {
			handler.ResponseErr(ctx, err)
			return
		}

	}

	handler.SendAudit(audit.ModuleMicroApplication, audit.ActionDelete+audit.ModuleServiceEntry, application+"/"+name, ctx)
	handler.ResponseOk(ctx, nil)
	return
}

func DomainValidationServiceEntry(mgr multiCluster.Manager, ctx iris.Context, host string) error {
	appCtx := handler.ExtractAppContext(ctx)
	domain := host

	var domainClient, err = mgr.DynamicClient(appCtx.ClusterName, DomianValidationGVK)
	if err != nil {
		logger.Errorf("Dynamic Client %+v, %s", *DomianValidationGVK, err)
		handler.RespondWithDetailedError(ctx, microapierrors.DynamicClientErr(err))
		return microapierrors.BadRequest("域名无效")
	}
	domainvalidation, err := domainClient.Get(context.TODO(), domain, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		} else {
			return err
		}
	}
	if domainvalidation != nil {
		if scope, ok := domainvalidation.GetLabels()[ScopLabel]; ok {
			if scope == IstioScope {
				return microapierrors.BadRequest(fmt.Sprintf("域名%s已被使用", host))
			}
		}

	}

	return nil
}

func createDomainValidation(mgr multiCluster.Manager, ctx iris.Context, host string) error {
	appCtx := handler.ExtractAppContext(ctx)
	domain := host

	var domainClient, err = mgr.DynamicClient(appCtx.ClusterName, DomianValidationGVK)
	if err != nil {
		logger.Errorf("Dynamic Client %+v, %s", *DomianValidationGVK, err)
		return err
	}
	labels := map[string]string{
		"tpaas.troila.com/domain": domain,
		ScopLabel:                 IstioScope,
	}
	domainvalidation := &infratroilacomv1beta1.ClusterDomainValidate{
		TypeMeta: metav1.TypeMeta{
			Kind:       common.DomainValidationKind,
			APIVersion: infratroilacomv1beta1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   domain,
			Labels: labels,
		},
		Spec: infratroilacomv1beta1.ClusterDomainValidateSpec{},
	}
	unObj, err := common.ConvertResourceToUnstructured(domainvalidation)
	if err != nil {
		logger.Errorf("ConvertResourceToUnstructured err %s", err)
		return err
	}
	_, err = domainClient.Create(context.TODO(), unObj, metav1.CreateOptions{})
	if err != nil {
		logger.Errorf("ConvertResourceToUnstructured err %s", err)
		return err
	}
	return nil
}

func RecoveryDomainValidation(mgr multiCluster.Manager, ctx iris.Context, host string) {
	appCtx := handler.ExtractAppContext(ctx)
	domain := host

	var domainClient, err = mgr.DynamicClient(appCtx.ClusterName, DomianValidationGVK)
	if err == nil {
		domainvalidation, err := domainClient.Get(context.TODO(), domain, metav1.GetOptions{})
		if err == nil {
			if domainvalidation.GetLabels() != nil {
				if v, ok := domainvalidation.GetLabels()[ScopLabel]; ok && v == IstioScope {
					domainClient.Delete(context.TODO(), domain, metav1.DeleteOptions{})
				}
			}
		}
	}
}
