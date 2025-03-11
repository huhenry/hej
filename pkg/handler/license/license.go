package license

import (
	"errors"

	"github.com/huhenry/hej/pkg/backend"
	customErrors "github.com/huhenry/hej/pkg/errors"
	"github.com/huhenry/hej/pkg/handler"
	"github.com/huhenry/hej/pkg/log"
	"github.com/kataras/iris/v12"
)

var logger = log.RegisterScope("license-filter")

const (
	StatusExpired   = -1
	StatusNoLicense = -2
	StatusOverCpu   = -3
)

func Handler() func(iris.Context) {
	return func(ctx iris.Context) {

		userContext := handler.ExtractUserContext(ctx)
		if userContext == nil {
			logger.Warnf("fail to extract user for license")
			handler.Response(ctx, customErrors.StatusCodeUnAuthorized, "未授权")
			return
		}
		code, err := ValidLicense(userContext.Token)
		if code != 200 {
			handler.Response(ctx, code, err.Error())
			return
		}

		ctx.Next()
	}
}

func ValidLicense(token string) (int, error) {
	license, _ := backend.GetClient().V1().License().
		Licenseinfo(token)

	if license == nil {
		return 200, nil
	}
	switch license.Status {
	case StatusExpired:
		return customErrors.StatusCodeLicenseInvalid, errors.New("license超期")
	case StatusNoLicense:
		return customErrors.StatusCodeLicenseInvalid, errors.New("license超期")
	case StatusOverCpu:
		return customErrors.StatusCodeLicenseOverload, errors.New("license资源超限")
	}

	return 200, nil
}
