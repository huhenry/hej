package microapp

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/huhenry/hej/pkg/microapp/resource"
	"github.com/huhenry/hej/pkg/registry"

	v1 "github.com/huhenry/hej/pkg/backend/v1"

	resource2 "github.com/huhenry/hej/pkg/microapp/resource"

	"github.com/kataras/iris/v12"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"

	"github.com/huhenry/hej/pkg/common/app"
	customErrors "github.com/huhenry/hej/pkg/errors"
	errors2 "github.com/huhenry/hej/pkg/errors"
	"github.com/huhenry/hej/pkg/handler"
	"github.com/huhenry/hej/pkg/handler/audit"
	"github.com/huhenry/hej/pkg/handler/auth"
	micro "github.com/huhenry/hej/pkg/microapp"
	"github.com/huhenry/hej/pkg/microapp/v1beta1"
	"github.com/huhenry/hej/pkg/multiCluster"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	domainComponent = `([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])`
	domain          = fmt.Sprintf(`localhost|(%s([.]%s)+)(:[0-9]+)?`, domainComponent, domainComponent)
	domainRegexp    = regexp.MustCompile(domain)

	PathParameterName     = "name"
	PathParameterNodeType = "node_type"
)

type MicroServiceCreation struct {
	ServiceName  string `json:"serviceName"`
	WorkloadName string `json:"workloadName"`
	Version      string `json:"version"`
}

func validateCreation(creation *MicroServiceCreation) error {
	if creation.ServiceName == "" {
		return errors2.BadRequest("缺少服务")
	}
	if creation.WorkloadName == "" {
		return errors2.BadRequest("缺少工作负载")
	}
	if creation.Version == "" {
		return errors2.BadRequest("缺少版本信息")
	}

	if !handler.IsDNS1123Label(creation.Version) {
		return errors2.BadRequest("版本信息中包含特殊字符")

	}

	return nil
}

type NodeStatus struct {
	Name     string      `json:"name,omitempty"`
	NodeType string      `json:"node_type,omitempty"`
	Status   interface{} `json:"status,omitempty"`
}

func Healthz(ctx iris.Context) {
	appCtx := handler.ExtractAppContext(ctx)

	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}

	name := ctx.Params().GetString(PathParameterName)
	if name == "" {

		handler.RespondWithDetailedError(ctx, customErrors.BadParametersErr(fmt.Errorf("缺少名称")))
		return
	}

	nodeType := ctx.Params().GetString(PathParameterNodeType)
	if nodeType == "" {

		handler.RespondWithDetailedError(ctx, customErrors.BadParametersErr(fmt.Errorf("缺少节点类型")))
		return
	}

	nodestatus := NodeStatus{
		Name:     name,
		NodeType: nodeType,
	}

	stats, err := micro.Resource().GetStats(appCtx.ClusterName, appCtx.AppId)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}

	switch strings.ToLower(nodeType) {
	case "service":
		//TODO fetchService
		entity, err := micro.MicroServiceEntry().Get(resource, name, "")
		if err != nil {
			handler.ResponseErr(ctx, err)
			return
		}
		status := ParseMicroserviceStatus(*entity, stats)
		nodestatus.Status = &status
		//service, err := stats.GetService(name)
		//paramQuery := handler.ExtractQueryParam(ctx)

	case "app":

		if workload, ok := stats.GetDeployment(name); ok {
			status := ParseWorkloadStatus(workload)
			nodestatus.Status = &status
		}

	}

	handler.ResponseOk(ctx, nodestatus)

}

func ParseWorkloadStatus(workload *v1.DeploymentResource) v1beta1.MicroServiceStatus {
	status := v1beta1.MicroServiceStatus{
		Phase: v1beta1.MicroServicePhaseRunning,
	}
	if workload.Config != nil {
		desiredReplicas := *workload.Config.Spec.Replicas
		availableReplicas := workload.Config.Status.AvailableReplicas

		status.DesiredReplicas = desiredReplicas
		status.ReadyReplicas = availableReplicas
		if desiredReplicas == 0 || availableReplicas == 0 {
			status.Phase = v1beta1.MicroServicePhaseWorkloadNoReplicas
			status.Conditions = []v1beta1.MicroServiceCondition{
				v1beta1.MicroServiceCondition{
					Message: v1beta1.ConditionPhaseTrans[v1beta1.MicroServicePhaseWorkloadNoReplicas],
				},
			}

		} else if desiredReplicas != availableReplicas {

			status.Phase = v1beta1.MicroServicePhaseProgressing

		}

	}
	return status

}

func buildMicroServiceEntity(ctx iris.Context, creation *MicroServiceCreation) *v1beta1.MicroServiceEntity {
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
	ms.Version = creation.Version
	ms.Workload = v1beta1.CrossNamespaceObjectReference{
		APIVersion: appv1.SchemeGroupVersion.String(),
		Kind:       "Deployment",
		Name:       creation.WorkloadName,
		Namespace:  appCtx.KubeNamespace,
	}

	return ms
}

func BatchCreateMicroService(ctx iris.Context) {
	var batch []MicroServiceCreation
	err := ctx.ReadJSON(&batch)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}

	entities := make([]v1beta1.MicroServiceEntity, 0)
	for i := range batch {
		creation := batch[i]
		if err = validateCreation(&creation); err != nil {
			handler.ResponseErr(ctx, err)
			return
		}
		ms := buildMicroServiceEntity(ctx, &creation)
		entities = append(entities, *ms)
	}

	var failServices []string
	for i := range entities {
		entity := entities[i]
		err := micro.MicroService().Create(&entity)
		err = convertCheckingBindingError(err)
		if err != nil {
			logger.Errorf("batch to create microservice failed, name:%s, cause:%v", entity.Name, err)
			failServices = append(failServices, entity.ServiceName)
		} else {
			handler.SendAudit(audit.ModuleMicroService, audit.ActionCreate, entity.Application+"/"+entity.Name, ctx)
		}
	}

	if len(failServices) > 0 {
		msg := "服务 " + strings.Join(failServices, ",") + " 添加失败"
		handler.Response(ctx, customErrors.StatusCodeServiceError, msg)
	} else {
		handler.ResponseOk(ctx, nil)
	}
}

func convertCheckingBindingError(err error) error {
	if err == nil {
		return nil
	}
	if err == resource.ErrServiceOccupied {
		return errors2.BadRequest("服务已被加入其它应用")
	}
	if err == resource.ErrDeploymentOccupied {
		return errors2.BadRequest("工作负载已被加入其它应用")
	}
	if err == resource.ErrServiceWorkloadNotMatch {
		return errors2.BadRequest("服务与工作负载不匹配")
	}
	if err == resource.ErrServiceMatchMultiWorkload {
		return errors2.BadRequest("服务关联了多个工作负载")
	}
	if err == resource.ErrDeploymentMatchMultiService {
		return errors2.BadRequest("工作负载关联了多个服务")
	}

	return err
}

func CreateMicroService(ctx iris.Context) {
	creation := &MicroServiceCreation{}

	err := ctx.ReadJSON(creation)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}
	if err = validateCreation(creation); err != nil {
		handler.ResponseErr(ctx, err)
		return
	}

	ms := buildMicroServiceEntity(ctx, creation)
	err = micro.MicroService().Create(ms)
	err = convertCheckingBindingError(err)
	if err != nil {
		handler.ResponseErr(ctx, err)
	} else {
		handler.SendAudit(audit.ModuleMicroService, audit.ActionCreate, ms.Application+"/"+ms.Name, ctx)
		handler.ResponseOk(ctx, nil)
	}
}

func DeleteMicroService(ctx iris.Context) {
	name := ctx.Params().GetString("name")
	application := ctx.Params().GetString("application")
	unbinding, err := ctx.URLParamBool("unbinding")
	if err != nil {
		unbinding = false
	}

	canAccess := false
	if unbinding {
		canAccess = auth.HasAuthorization(ctx, auth.SU, auth.DU)
	} else {
		canAccess = auth.HasAuthorization(ctx, auth.SD, auth.DD)
	}
	if !canAccess {
		handler.Response(ctx, customErrors.StatusCodeUnAuthorized, "未授权")
		return
	}

	appCtx := handler.ExtractAppContext(ctx)
	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}

	err = micro.MicroService().Delete(resource, name, unbinding)
	if err != nil {
		handler.ResponseErr(ctx, err)
	} else {
		if unbinding {
			handler.SendAudit(audit.ModuleMicroService, audit.ActionUnbinding, application+"/"+name, ctx)
		} else {
			handler.SendAudit(audit.ModuleMicroService, audit.ActionDelete, application+"/"+name, ctx)
		}
		handler.ResponseOk(ctx, nil)
	}
}

type PartialWorkload struct {
	Name       string                     `json:"name"`
	Replicas   int32                      `json:"replicas"`
	Containers []PartialWorkloadContainer `json:"containers"`
}

type PartialWorkloadContainer struct {
	Name       string      `json:"name"`
	Image      string      `json:"image"`
	Tag        string      `json:"tag"`
	Credential *Credential `json:"cred,omitempty""`
}
type Credential struct {
	Host     string `json:"host"`
	UserName string `json:"user"`
	Password string `json:"password"`
}

func GetWorkloadContainers(mgr multiCluster.Manager, ctx iris.Context) {
	name := ctx.Params().GetString("name")
	application := ctx.Params().GetString("application")

	appCtx := handler.ExtractAppContext(ctx)
	//userCtx := handler.ExtractUserContext(ctx)
	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}
	k8sClient, err := mgr.Client(appCtx.ClusterName)
	if err != nil {
		msg := fmt.Sprintf("集群连接失败 : %s", err)
		handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, msg)

		return
	}

	ms, err := micro.MicroService().GetEntity(resource, application, name)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}

	if ms.Workload.Name == "" {
		logger.Warnf("workload[%s] name is missing \r\n", ms.Workload.Name)
		handler.ResponseOk(ctx, nil)
		return
	}

	workload, err := micro.Resource().GetDeployment(appCtx.ClusterName, appCtx.AppId, appCtx.KubeNamespace, ms.Workload.Name)
	/*
		backend.GetClient().V1().Deployment().
	Get(appCtx.ClusterName, appCtx.AppId, userCtx.Token, ms.Workload.Name)*/
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}

	pw := &PartialWorkload{
		Name:       ms.Workload.Name,
		Containers: make([]PartialWorkloadContainer, 0),
	}
	if workload.Config != nil {
		if workload.Config.Spec.Replicas != nil {
			pw.Replicas = *workload.Config.Spec.Replicas
		}
		creds := ParseSecrets(k8sClient, workload.Config.Spec.Template, workload.Config.Namespace)
		for _, c := range workload.Config.Spec.Template.Spec.Containers {
			p := PartialWorkloadContainer{
				Name: c.Name,
			}
			index := strings.LastIndex(c.Image, ":")
			if index > 0 {
				p.Image = c.Image[:index]
				p.Tag = c.Image[index+1:]

			} else {
				p.Image = c.Image
			}

			p.Credential = parseCreditient(p.Image, creds)

			pw.Containers = append(pw.Containers, p)
		}
	}

	handler.ResponseOk(ctx, pw)
}

func parseCreditient(image string, creds registry.Credentials) *Credential {

	domain := parseDomain(image)
	//if domain
	if len(domain) == 0 {
		domain = "docker.io"
	}

	if cred := creds.CredsFor(domain); cred != nil {
		return &Credential{
			Host:     cred.Registry,
			UserName: cred.Username,
			Password: cred.Password,
		}

	}

	return nil

}

func parseDomain(s string) string {
	elements := strings.Split(s, "/")
	switch len(elements) {
	case 0, 1:
		return ""
	case 2:
		if domainRegexp.MatchString(elements[0]) {
			return elements[0]
		}
		return ""
	default:
		return elements[0]
	}
	return ""
}

/*

	switch len(elements) {
	case 0: // NB strings.Split will never return []
		return id, errors.Wrapf(ErrMalformedImageID, "parsing %q", s)
	case 1: // no slashes, e.g., "alpine:1.5"; treat as library image
		id.Image = s
	case 2: // may have a domain e.g., "localhost/foo", or not e.g., "weaveworks/scope"
		if domainRegexp.MatchString(elements[0]) {
			id.Domain = elements[0]
			id.Image = elements[1]
		} else {
			id.Image = s
		}
	default: // cannot be a library image, so the first element is assumed to be a domain
		id.Domain = elements[0]
		id.Image = strings.Join(elements[1:], "/")
	}

	// Figure out if there's a tag
	imageParts := strings.Split(id.Image, ":")
	switch len(imageParts) {
	case 1:
		break
	case 2:
		if imageParts[0] == "" || imageParts[1] == "" {
			return id, errors.Wrapf(ErrMalformedImageID, "parsing %q", s)
		}
		id.Image = imageParts[0]
		id.Tag = imageParts[1]
	default:
		return id, ErrMalformedImageID
	}

	return id, nil
}
*/

func ParseSecrets(k8sclient kubernetes.Interface, podTemplate corev1.PodTemplateSpec, namespace string) registry.Credentials {

	seenCreds := make(map[string]registry.Credentials)

	creds := registry.NoCredentials()
	for _, imagePullSecret := range podTemplate.Spec.ImagePullSecrets {
		name := imagePullSecret.Name
		if seen, ok := seenCreds[name]; ok {
			creds.Merge(seen)
			continue
		}

		secret, err := k8sclient.CoreV1().Secrets(namespace).Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			logger.Warn(errors.Wrapf(err, "getting secret %q from namespace %q", name, namespace))

			seenCreds[name] = registry.NoCredentials()
			continue
		}

		var decoded []byte
		var ok bool
		// These differ in format; but, ParseCredentials will
		// handle either.
		switch corev1.SecretType(secret.Type) {
		case corev1.SecretTypeDockercfg:
			decoded, ok = secret.Data[corev1.DockerConfigKey]
		case corev1.SecretTypeDockerConfigJson:
			decoded, ok = secret.Data[corev1.DockerConfigJsonKey]
		default:
			logger.Warn("skip", "unknown type", "secret", namespace+"/"+secret.Name, "type", secret.Type)

			seenCreds[name] = registry.NoCredentials()
			continue
		}

		if !ok {
			logger.Warn(errors.Wrapf(err, "retrieving pod secret %q", secret.Name))

			seenCreds[name] = registry.NoCredentials()
			continue
		}

		// Parse secret
		crd, err := registry.ParseCredentials(fmt.Sprintf("%s:secret/%s", namespace, name), decoded)
		if err != nil {
			logger.Error(errors.Wrapf(err, "ParseCredentials %s", decoded))
			seenCreds[name] = registry.NoCredentials()
			continue
		}

		seenCreds[name] = crd

		// Merge into the credentials for this PodSpec
		creds.Merge(crd)
	}
	return creds
}

func GetAvailableWorkloads(ctx iris.Context) {
	name := ctx.Params().GetString("name")
	application := ctx.Params().GetString("application")

	appCtx := handler.ExtractAppContext(ctx)
	userCtx := handler.ExtractUserContext(ctx)

	wls := make([]map[string]interface{}, 0)

	if !auth.CheckPermissions(appCtx.NamespaceId, appCtx.AppId, userCtx) {

		handler.ResponseOk(ctx, wls)
		return
	}

	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}
	_, err := micro.MicroService().GetEntity(resource, application, name)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}

	stats, err := micro.Resource().GetStats(appCtx.ClusterName, appCtx.AppId)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}

	var toMap = func(r *v1.DeploymentResource) map[string]interface{} {
		m := make(map[string]interface{})
		m["name"] = r.Name
		m["desiredReplicas"] = r.Config.Spec.Replicas
		m["availableReplicas"] = r.AvailableReplicas

		return m
	}

	for _, deploy := range stats.FreeDeployments(resource2.DeployNonOccupied, resource2.DeployLabelsFilter) {
		wls = append(wls, toMap(deploy))
	}
	deploys := stats.SvcDeployments(name, resource2.DeployNonOccupied)
	for _, deploy := range deploys {
		if stats.DeploySvcCount(deploy.Name) == 1 { // only this service
			wls = append(wls, toMap(deploy))
		}
	}

	handler.ResponseOk(ctx, wls)
}
