package microapp

import (
	"context"
	"sort"
	"strings"

	"github.com/huhenry/hej/pkg/common/dynamic"
	"github.com/huhenry/hej/pkg/hash"

	"github.com/kataras/iris/v12"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/huhenry/hej/pkg/common"
	"github.com/huhenry/hej/pkg/common/app"
	"github.com/huhenry/hej/pkg/common/page"
	"github.com/huhenry/hej/pkg/define"
	customErrors "github.com/huhenry/hej/pkg/errors"
	errors2 "github.com/huhenry/hej/pkg/errors"
	"github.com/huhenry/hej/pkg/handler"
	"github.com/huhenry/hej/pkg/handler/audit"
	micro "github.com/huhenry/hej/pkg/microapp"
	"github.com/huhenry/hej/pkg/microapp/v1beta1"
	"github.com/huhenry/hej/pkg/multiCluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	infratroilacomv1beta1 "paas.gitlab.com/paas/cluster_config/api/v1beta1"
)

const (
	ScopLabel  = "tpaas.troila.com/scope"
	IstioScope = "istio"
	MaxLength  = 63
)

var DomianValidationGVK = &schema.GroupVersionKind{
	Group:   infratroilacomv1beta1.GroupVersion.Group,
	Version: infratroilacomv1beta1.GroupVersion.Version,
	Kind:    common.DomainValidationKind,
}

var domianGVK = &schema.GroupVersionKind{
	Group:   infratroilacomv1beta1.GroupVersion.Group,
	Version: infratroilacomv1beta1.GroupVersion.Version,
	Kind:    common.DomainKind,
}

var gatewayGVK = &schema.GroupVersionKind{
	Group:   "microservices.troila.com",
	Version: "v1beta1",
	Kind:    "Gateway",
}

func validateGateway(gw *v1beta1.GatewayEntity) error {
	if gw.Domain == "" {
		return errors2.BadRequest("缺少域名")
	}
	if len(gw.Domain) > MaxLength {
		return errors2.BadRequest("域名长度不能超过63个字符")

	}
	if gw.ServiceName == "" {
		return errors2.BadRequest("缺少服务")
	}
	if gw.Port <= 0 {
		return errors2.BadRequest("端口号非法")
	}
	if gw.Protocol == "" {
		return errors2.BadRequest("缺少协议")
	}
	if gw.Protocol != "HTTP" && gw.Protocol != "HTTPS" {
		return errors2.BadRequest("协议必须是HTTP或HTTPS")
	}
	if gw.Path == "" {
		return errors2.BadRequest("缺少路径")
	} else if gw.Path[0] != '/' {
		return errors2.BadRequest("路径必须以/开头")
	}
	if gw.Protocol == "HTTPS" && (gw.Secret == "" && gw.DomainSecret == "") {
		return errors2.BadRequest("缺少密钥")
	}

	if len(gw.ClusterDomain) > 0 && len(gw.ClusterDomain)+len(gw.Domain) > MaxLength-1 {

		return errors2.BadRequest("域名长度不能超过63个字符")
	}

	if len(gw.DomainSecret) > 0 && !strings.Contains(gw.DomainSecret, "/") {
		return errors2.BadRequest("集群密钥缺少信息")
	}

	if gw.Rewrite != "" && gw.Rewrite[0] != '/' {
		return errors2.BadRequest("地址重写必须以/开头")
	}

	return nil
}

func BatchCreateGateway(mgr multiCluster.Manager, ctx iris.Context) {
	var gateway v1beta1.GatewayTransModel
	err := ctx.ReadJSON(&gateway)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}

	appCtx := handler.ExtractAppContext(ctx)
	userCtx := handler.ExtractUserContext(ctx)
	var failGateways []string
	for i := range gateway.Services {
		gw := GenGateway(&gateway, gateway.Services[i])
		if err = validateGateway(gw); err != nil {
			handler.ResponseErr(ctx, err)
			return
		}

		gw.AppResources = app.AppResources{
			KubeNamespace: appCtx.KubeNamespace,
			Cluster:       appCtx.ClusterName,
			AppId:         appCtx.AppId,
			NamespaceId:   appCtx.NamespaceId,
		}
		gw.Creator = userCtx.Name
		gw.Application = ctx.Params().GetString("application")

		if err = DomainValidation(mgr, ctx, gw); err != nil {
			handler.ResponseErr(ctx, err)
			return
		}

		err := micro.Gateway().Create(gw)
		if err != nil {
			logger.Errorf("batch to create gateway failed, name:%s, cause:%v", gw.Name, err)
			failGateways = append(failGateways, gw.GetDomain()+gw.Path)
		} else {
			RegisteDomainValidation(mgr, ctx, gw)
			handler.SendAudit(audit.ModuleGateway, audit.ActionCreate, gw.AuditMessage(), ctx)
		}
	}

	if len(failGateways) > 0 {
		msg := "网关 " + strings.Join(failGateways, ",") + " 创建失败"
		handler.Response(ctx, customErrors.StatusCodeServiceError, msg)
	} else {
		handler.ResponseOk(ctx, nil)
	}

}

func GenGateway(transModel *v1beta1.GatewayTransModel, service *v1beta1.GatewayService) *v1beta1.GatewayEntity {

	gatewayEntity := &v1beta1.GatewayEntity{}
	gatewayEntity.GatewaySpec = v1beta1.GatewaySpec{
		Domain:        transModel.Domain,
		Protocol:      transModel.Protocol,
		Path:          service.Path,
		Port:          service.Port,
		ServiceName:   service.ServiceName,
		Rewrite:       service.Rewrite,
		Secret:        transModel.Secret,
		ClusterDomain: transModel.ClusterDomain,
		DomainSecret:  transModel.DomainSecret,
	}

	return gatewayEntity

}

func CreateGateway(ctx iris.Context) {
	gw := &v1beta1.GatewayEntity{}
	err := ctx.ReadJSON(gw)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}
	if err = validateGateway(gw); err != nil {
		handler.ResponseErr(ctx, err)
		return
	}

	appCtx := handler.ExtractAppContext(ctx)
	userCtx := handler.ExtractUserContext(ctx)
	gw.AppResources = app.AppResources{
		KubeNamespace: appCtx.KubeNamespace,
		Cluster:       appCtx.ClusterName,
		AppId:         appCtx.AppId,
		NamespaceId:   appCtx.NamespaceId,
	}
	gw.Creator = userCtx.Name

	gw.Application = ctx.Params().GetString("application")

	err = micro.Gateway().Create(gw)
	if err != nil {
		handler.ResponseErr(ctx, err)
	} else {
		handler.SendAudit(audit.ModuleGateway, audit.ActionCreate, gw.AuditMessage(), ctx)
		handler.ResponseOk(ctx, nil)
	}
}

func DeleteGateway(mgr multiCluster.Manager, ctx iris.Context) {
	name := ctx.Params().GetString("name")
	application := ctx.Params().GetString("application")
	appCtx := handler.ExtractAppContext(ctx)
	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}
	gw, err := micro.Gateway().Get(resource, application, name)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}

	err = micro.Gateway().Delete(resource, application, name)
	if err != nil {
		handler.ResponseErr(ctx, err)
	} else {

		DropDomainValidation(mgr, ctx, gw)
		handler.SendAudit(audit.ModuleGateway, audit.ActionDelete, gw.AuditMessage(), ctx)
		handler.ResponseOk(ctx, nil)
	}
}

func ListGateway(ctx iris.Context) {
	application := ctx.Params().GetString("application")
	appCtx := handler.ExtractAppContext(ctx)
	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}
	paramQuery := handler.ExtractQueryParam(ctx)

	gateways, err := micro.Gateway().List(resource, application)
	if err != nil {
		handler.ResponseErr(ctx, err)
	} else {
		sort.Slice(gateways, func(i, j int) bool {
			r := gateways[i].CreateTimeSec > gateways[j].CreateTimeSec
			if paramQuery.Sortby == define.SortByCreateTime {
				r = !r
			}

			return r
		})

		data := make([]interface{}, 0)
		for i := range gateways {
			data = append(data, gateways[i])
		}

		handler.ResponseOk(ctx, page.PageInfo(data, paramQuery))
	}
}
func DropDomainValidation(mgr multiCluster.Manager, ctx iris.Context, gw *v1beta1.GatewayEntity) error {
	appCtx := handler.ExtractAppContext(ctx)
	domainPath := gw.GetDomainHashPath()
	domainClient, err := mgr.DynamicClient(appCtx.ClusterName, DomianValidationGVK)
	if err != nil {
		logger.Errorf("Dynamic Client %+v, %s", *DomianValidationGVK, err)
		return errors2.CustomClientErr("删除失败", err)
	}

	err = domainClient.Delete(context.TODO(), domainPath, metav1.DeleteOptions{})
	if err != nil {
		logger.Errorf("Dynamic Client %+v, %s", *DomianValidationGVK, err)
		return errors2.CustomClientErr("删除失败", err)
	}
	return nil

}

func DomainValidation(mgr multiCluster.Manager, ctx iris.Context, gw *v1beta1.GatewayEntity) error {

	appCtx := handler.ExtractAppContext(ctx)
	domain := gw.GetDomain()
	domainPath := gw.GetDomainHashPath()

	if len(gw.ClusterDomain) > 0 {
		domainClient, err := mgr.DynamicClient(appCtx.ClusterName, domianGVK)
		if err != nil {
			logger.Errorf("Dynamic Client %+v, %s", *DomianValidationGVK, err)

			return errors2.BadRequest("集群域名无效")

		}
		clusterDomainUN, err := domainClient.Get(context.TODO(), gw.ClusterDomain, metav1.GetOptions{})

		if err != nil {
			return errors2.BadRequest("集群域名无效")
		}

		clusterDomain := &infratroilacomv1beta1.ClusterDomain{}

		err = dynamic.FromUnstructured(clusterDomainUN, clusterDomain)
		if err != nil {

			logger.Errorf("FromUnstructured to clusterDomain failed  %s", err)
			return errors2.BadRequest("集群域名无效")
		}
		if clusterDomain != nil && clusterDomain.Spec.Scope != infratroilacomv1beta1.ScopeIstio {

			return errors2.BadRequest("集群域名无效")
		}

	}

	domainClient, err := mgr.DynamicClient(appCtx.ClusterName, DomianValidationGVK)
	if err != nil {
		logger.Errorf("Dynamic Client %+v, %s", *DomianValidationGVK, err)

		handler.RespondWithDetailedError(ctx, errors2.DynamicClientErr(err))
		return errors2.BadRequest("域名无效")

	}
	domainvalidation, err := domainClient.Get(context.TODO(), domain, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {

			return nil

		}

	}

	if domainvalidation != nil {
		if scope, ok := domainvalidation.GetLabels()[ScopLabel]; ok && scope != IstioScope {

			return errors2.BadRequest("域名已被使用")

		}

	}

	domainPathvalidation, err := domainClient.Get(context.TODO(), domainPath, metav1.GetOptions{})
	if err != nil && k8serrors.IsNotFound(err) {
		return nil
	}
	if domainPathvalidation != nil {

		return errors2.BadRequest("域名路径已被使用")

	}

	return nil

}

func RegisteDomainValidation(mgr multiCluster.Manager, ctx iris.Context, gw *v1beta1.GatewayEntity) error {

	appCtx := handler.ExtractAppContext(ctx)
	domain := gw.GetDomain()
	hashPath := hash.HashToString(gw.Path)
	domainPath := gw.GetDomainHashPath()

	domainClient, err := mgr.DynamicClient(appCtx.ClusterName, DomianValidationGVK)
	if err != nil {
		logger.Errorf("Dynamic Client %+v, %s", *DomianValidationGVK, err)

		//handler.RespondWithDetailedError(ctx, errors2.DynamicClientErr(err))
		return errors2.BadRequest("域名无效")

	}

	labels := map[string]string{
		"tpaas.troila.com/domain":      domain,
		"tpaas.troila.com/domain.path": hashPath,
		ScopLabel:                      IstioScope,
	}
	if len(gw.ClusterDomain) > 0 {
		labels["tpaas.troila.com/clusterdomain"] = gw.ClusterDomain
	}

	annotations := map[string]string{
		"tpaas.troila.com/domain.path": gw.Path,
	}
	owner, err := GenOwner(gw)
	if err != nil {

		logger.Errorf("Gen Owner error  %+v, %s", gw, err)
		return err
	}

	_, err = domainClient.Get(context.TODO(), domain, metav1.GetOptions{})
	if err != nil && k8serrors.IsNotFound(err) {

		domainvalidation := &infratroilacomv1beta1.ClusterDomainValidate{
			TypeMeta: metav1.TypeMeta{
				Kind:       common.DomainValidationKind,
				APIVersion: infratroilacomv1beta1.GroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        domain,
				Labels:      labels,
				Annotations: annotations,
			},
			Spec: infratroilacomv1beta1.ClusterDomainValidateSpec{},
		}
		unObj, err := common.ConvertResourceToUnstructured(domainvalidation)
		if err != nil {
			logger.Errorf("ConvertResourceToUnstructured err %s", err)
			return errors2.BadRequest("域名无效")
		}

		_, err = domainClient.Create(context.TODO(), unObj, metav1.CreateOptions{})
		if err != nil {

			logger.Errorf("ConvertResourceToUnstructured err %s", err)
			return errors2.BadRequest("域名已被使用")
		}

	}

	pathValidation := &infratroilacomv1beta1.ClusterDomainValidate{
		TypeMeta: metav1.TypeMeta{
			Kind:       common.DomainValidationKind,
			APIVersion: infratroilacomv1beta1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            domainPath,
			Labels:          labels,
			Annotations:     annotations,
			OwnerReferences: []metav1.OwnerReference{*owner},
		},
		Spec: infratroilacomv1beta1.ClusterDomainValidateSpec{},
	}
	pathUnObj, err := common.ConvertResourceToUnstructured(pathValidation)
	if err != nil {
		logger.Errorf("ConvertResourceToUnstructured err %s", err)
		return errors2.BadRequest("域名路径已被使用")
	}

	_, err = domainClient.Create(context.TODO(), pathUnObj, metav1.CreateOptions{})
	if err != nil {

		logger.Errorf("pathValidation err %s", err)
		return errors2.BadRequest("域名路径已被使用")
	}

	return nil

}

func GenOwner(gw *v1beta1.GatewayEntity) (*metav1.OwnerReference, error) {

	gtw, err := micro.Gateway().FetchGateway(gw.AppResources, gw.Name)
	if err != nil {

		logger.Errorf("FetchGateway error  %+v, %s", gw, err)
		return nil, errors2.BadRequest("找不到Gateway资源")
	}

	blockOwnerDeletion := false
	isController := false

	owner := &metav1.OwnerReference{
		APIVersion:         gatewayGVK.GroupVersion().String(),
		Kind:               gatewayGVK.Kind,
		Name:               gtw.GetName(),
		UID:                gtw.GetUID(),
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}
	return owner, nil
}
