package bookinfo

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/huhenry/hej/pkg/handler/audit"

	"github.com/huhenry/hej/pkg/common/translate"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	customErrors "github.com/huhenry/hej/pkg/errors"

	"github.com/huhenry/hej/pkg/clusterConfig"
	"github.com/huhenry/hej/pkg/handler"
	"github.com/huhenry/hej/pkg/installation/bookinfo"
	"github.com/huhenry/hej/pkg/multiCluster"
	"github.com/kataras/iris/v12"
)

func InstallHandler(mgr multiCluster.Manager) func(ctx iris.Context) {
	return func(ctx iris.Context) {
		Install(mgr, ctx)
	}
}

func Install(mgr multiCluster.Manager, ctx iris.Context) {
	appCtx := handler.ExtractAppContext(ctx)
	userCtx := handler.ExtractUserContext(ctx)
	m := make(map[string]string)
	ctx.ReadJSON(&m)

	name := m["name"]
	if name == "" {
		handler.Response(ctx, http.StatusBadRequest, "缺少名称")
		return
	}

	harbor, err := clusterConfig.GetHarbor(mgr, appCtx.ClusterName)
	if err != nil {
		handler.ResponseErr(ctx, err)
		return
	}

	params := bookinfo.Params{
		Cluster:      appCtx.ClusterName,
		NamespaceId:  appCtx.NamespaceId,
		Namespace:    appCtx.KubeNamespace,
		AppId:        appCtx.AppId,
		AppName:      appCtx.AppName,
		Creator:      userCtx.Name,
		MicroAppName: name,
		Hub:          harbor,
	}
	dao := &bookinfo.DefaultDao{
		Mgr: mgr,
	}
	installer := bookinfo.NewInstaller(params, &bookinfo.ManifestBuilder{}, dao)
	context := context.Background()

	err = installer.Install(context)

	if err != nil {
		if existErr, ok := err.(*bookinfo.ResourceAlreadyExistErr); ok {
			msgs := make([]string, 0)
			for _, resource := range existErr.Resource {
				msgs = append(msgs, buildExistsError(resource))
			}

			msg := strings.Join(msgs, " ")
			handler.Response(ctx, customErrors.StatusCodeUnProcessableEntity, msg)
		} else {
			handler.ResponseErr(ctx, err)
		}

		return
	}

	handler.SendAudit(audit.ModuleMicroApplication, audit.ActionCreate, name, ctx)
	handler.ResponseOk(ctx, nil)
}

func buildExistsError(resource *unstructured.Unstructured) string {
	return fmt.Sprintf("%s %s在集群中已存在", translate.ToChinese(resource.GetObjectKind().GroupVersionKind()), resource.GetName())
}
