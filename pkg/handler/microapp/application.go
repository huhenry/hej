package microapp

import (
	"sort"

	"github.com/huhenry/hej/pkg/common/app"
	"github.com/huhenry/hej/pkg/common/page"
	"github.com/huhenry/hej/pkg/define"
	customerrors "github.com/huhenry/hej/pkg/errors"
	"github.com/huhenry/hej/pkg/handler"
	"github.com/huhenry/hej/pkg/handler/audit"
	"github.com/huhenry/hej/pkg/handler/microapp/filter"
	micro "github.com/huhenry/hej/pkg/microapp"
	"github.com/huhenry/hej/pkg/microapp/v1beta1"
	"github.com/kataras/iris/v12"
	"k8s.io/apimachinery/pkg/api/errors"
)

func CreateApplication(ctx iris.Context) {
	microapp := &v1beta1.MicroApp{}
	err := ctx.ReadJSON(microapp)
	if err != nil {
		handler.RespondWithDetailedError(ctx, customerrors.BadParametersErr(err))
		return
	}
	if microapp.Name == "" {
		handler.ResponseErr(ctx, errors.NewBadRequest("缺少应用名称"))
		return
	}

	appCtx := handler.ExtractAppContext(ctx)
	userCtx := handler.ExtractUserContext(ctx)
	microapp.AppId = appCtx.AppId
	microapp.Cluster = appCtx.ClusterName
	microapp.KubeNamespace = appCtx.KubeNamespace
	microapp.Creator = userCtx.Name
	microapp.NamespaceId = appCtx.NamespaceId

	err = micro.MicroApplicaiton().Create(microapp)
	if err != nil {

		handler.RespondWithDetailedError(ctx, customerrors.CustomClientErr("创建应用失败！", err))
	} else {
		handler.SendAudit(audit.ModuleMicroApplication, audit.ActionCreate, microapp.Name, ctx)
		handler.ResponseOk(ctx, nil)
	}
}

func GetApplication(ctx iris.Context) {
	name := ctx.Params().GetString("name")
	appCtx := handler.ExtractAppContext(ctx)
	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
	}

	result, err := micro.MicroApplicaiton().Get(resource, name)
	if err != nil {

		handler.RespondWithDetailedError(ctx, customerrors.CustomClientErr("获取应用失败！", err))

		return
	}

	dto := getMicroappDTO(result)

	handler.ResponseOk(ctx, dto)
}

func UpdateApplication(ctx iris.Context) {
	name := ctx.Params().GetString("name")
	data := make(map[string]string)
	err := ctx.ReadJSON(&data)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}
	desc := data["description"]
	appCtx := handler.ExtractAppContext(ctx)
	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}

	err = micro.MicroApplicaiton().Update(resource, name, desc)
	if err != nil {

		handler.RespondWithDetailedError(ctx, customerrors.CustomClientErr("更新应用失败！", err))
	} else {
		handler.SendAudit(audit.ModuleMicroApplication, audit.ActionUpdate, name, ctx)
		handler.ResponseOk(ctx, nil)
	}
}

func DeleteApplication(ctx iris.Context) {
	name := ctx.Params().GetString("name")
	appCtx := handler.ExtractAppContext(ctx)
	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}
	microapp := &v1beta1.MicroApp{}
	microapp.Name = name

	err := micro.MicroApplicaiton().Delete(resource, microapp)
	if err != nil {
		handler.ResponseErr(ctx, err)
	} else {
		handler.SendAudit(audit.ModuleMicroApplication, audit.ActionDelete, name, ctx)
		handler.ResponseOk(ctx, nil)
	}
}

/*
func ListApplicationBefore(ctx iris.Context) {
	appCtx := handler.ExtractAppContext(ctx)
	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}
	paramQuery := handler.ExtractQueryParam(ctx)

	all, err := micro.MicroApplicaiton().List(resource)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}

	microapps := make([]v1beta1.MicroApp, 0, paramQuery.PageSize)
	if paramQuery.Name == "" {
		microapps = all
	} else {
		for _, m := range all {
			microapp := m
			if strings.Contains(microapp.Name, paramQuery.Name) {
				microapps = append(microapps, m)
			}
		}
	}

	sort.Slice(microapps, func(i, j int) bool {
		r := microapps[i].CreateTimeSec > microapps[j].CreateTimeSec
		if paramQuery.Sortby == define.SortByCreateTime {
			r = !r
		}

		return r
	})
	data := make([]interface{}, 0)
	for i := range microapps {
		dto := getMicroappDTO(&microapps[i])
		data = append(data, dto)
	}
	handler.ResponseOk(ctx, page.PageInfo(data, paramQuery))
}
*/

func ListApplication(ctx iris.Context) {

	appCtx := handler.ExtractAppContext(ctx)
	paramQuery := handler.ExtractQueryParam(ctx)
	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
	}

	microAppAll, err := micro.MicroApplicaiton().List(resource)
	if err != nil {

		handler.RespondWithDetailedError(ctx, customerrors.CustomClientErr("获取所有应用失败！", err))

		return
	}

	/* microApps := microAppAll
	filters := filter.PraserParams(ctx)
	//通过过滤器层层过滤请求参数
	for _, filter := range filters {
		microApps = filter.Filte(microApps)
	} */
	microApp := &v1beta1.MicroApp{}
	filters := filter.ParserParams(ctx)
	microApps := make([]v1beta1.MicroApp, 0)
	for _, App := range microAppAll {
		microApp = &App
		for _, filter := range filters {
			microApp = filter.Filte(microApp)
		}

		if microApp != nil {
			microApps = append(microApps, *microApp)
		}

	}

	sort.Slice(microApps, func(i, j int) bool {
		r := microApps[i].CreateTimeSec > microApps[j].CreateTimeSec
		if paramQuery.Sortby == define.SortByCreateTime {
			r = !r
		}

		return r
	})
	data := make([]interface{}, 0)
	for i := range microApps {
		dto := getMicroappDTO(&microApps[i])
		data = append(data, dto)
	}
	logger.Infof("listApplication返回数据 ：%s", data)
	handler.ResponseOk(ctx, page.PageInfo(data, paramQuery))

}

type MicroappDTO struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Services    int32  `json:"services"`
	app.CreationInfo
	DesiredReplicas int32 `json:"desiredReplicas"`
	ReadyReplicas   int32 `json:"readyReplicas"`
}

func getMicroappDTO(microapp *v1beta1.MicroApp) *MicroappDTO {
	dto := &MicroappDTO{
		Name:            microapp.Name,
		Description:     microapp.Description,
		DesiredReplicas: microapp.DesiredReplicas,
		ReadyReplicas:   microapp.ReadyReplicas,
		Services:        microapp.BindingServices,
	}
	dto.Creator = microapp.Creator
	dto.CreateTimeSec = microapp.CreateTimeSec

	return dto
}

func ListApplicationServiceNames(ctx iris.Context) {
	application := ctx.Params().GetString("application")
	appCtx := handler.ExtractAppContext(ctx)
	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}

	type appService struct {
		Name           string   `json:"name,omitempty"`
		AppProtocol    []string `json:"appProtocol,omitempty"`
		IsServiceEntry bool     `json:"isServiceEntry"`
	}

	names := make([]appService, 0)

	microservices, err := micro.MicroService().List(resource, application, false)
	if err != nil && !errors.IsNotFound(err) {
		logger.Errorf("get microservices failed %v", err)
		handler.ResponseOk(ctx, names)
	} else {
		sort.Slice(microservices, func(i, j int) bool {
			r := microservices[i].CreateTimeSec > microservices[j].CreateTimeSec
			return r
		})
		for i := range microservices {

			isServiceEntry := false
			if microservices[i].ServiceEntry != nil && microservices[i].ServiceEntry.Hosts != nil && len(microservices[i].ServiceEntry.Hosts) > 0 {
				isServiceEntry = true
			}

			names = append(names, appService{
				Name:           microservices[i].ServiceName,
				AppProtocol:    microservices[i].GetAppProtocols(),
				IsServiceEntry: isServiceEntry,
			})

		}
		handler.ResponseOk(ctx, names)
	}
}

func ListApplicationNames(ctx iris.Context) {
	appCtx := handler.ExtractAppContext(ctx)
	resource := app.AppResources{
		AppId:         appCtx.AppId,
		Cluster:       appCtx.ClusterName,
		KubeNamespace: appCtx.KubeNamespace,
		NamespaceId:   appCtx.NamespaceId,
	}

	names := make([]string, 0)

	list, err := micro.MicroApplicaiton().List(resource)
	if err != nil {
		handler.ResponseErr(ctx, err)
	} else {
		for i := range list {
			names = append(names, list[i].Name)
		}
		handler.ResponseOk(ctx, names)
	}
}
