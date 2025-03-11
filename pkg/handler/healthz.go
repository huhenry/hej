package handler

import (
	"github.com/huhenry/hej/pkg/version"
	"github.com/kataras/iris/v12"

	"github.com/huhenry/hej/pkg/prometheusmetrics"
)

func Healthz(ctx iris.Context) {
	info := version.Get()

	prometheusmetrics.SetAppHealthyChecks(info.App, info.GitCommit, info.GitVersion, info.BuildDate, info.GoVersion)

	ResponseOk(ctx, info)

}
