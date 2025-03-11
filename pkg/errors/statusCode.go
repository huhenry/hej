package errors

/*

http://172.27.124.201:8090/pages/viewpage.action?pageId=5736835
200   正常返回

201 created  - 成功创建。

301 moved permanently - 请求内容已永久性转移到其他新位置。

302 found - 请求内容临时性转移到其他新位置

304 not modified   - HTTP缓存有效。

400  请求错误  一般为验证失败或者参数错误等异常，通常为系统可捕获的异常。

401 unauthorized   - 未授权（无权限）

403   权限异常  用户非法访问，权限错误。

404   请求资源不存在。

405 method not allowed - 该http方法不被允许。

415 unsupported media type - 请求类型错误。

422 unprocessable entity - 校验错误时用。

429 too many request - 请求过多。

500   内部错误  系统内部错误，例如未知错误。例如kubernetes服务暂停服务（业务错误）。
*/

const (
	// 201 created  - 成功创建。
	StatusCodeCreated int = 201

	// 301 moved permanently - 请求内容已永久性转移到其他新位置。
	StatusCodeMovedPermanently int = 301

	// 302 found - 请求内容临时性转移到其他新位置
	StatusCodeFound int = 302

	// 304 not modified   - HTTP缓存有效。
	StatusCodeNotModified int = 304

	// StatusCodeHTTPRequestErrorCode 400  请求错误  一般为验证失败或者参数错误等异常，通常为系统可捕获的异常。
	StatusCodeHTTPRequestErrorCode int = 400

	// 401 unauthorized   - 未授权（无权限）
	StatusCodeUnAuthorized int = 401

	// 403   权限异常  用户非法访问，权限错误。
	StatusCodeUserAccountException int = 403

	// 404   请求资源不存在。
	StatusCodeResourceNotFound int = 404

	// 415 unsupported media type - 请求类型错误。
	StatusCodeUnsupportedMediaType int = 415

	// StatusCodeUnProcessableEntity 422 unprocessable entity - 校验错误时用。
	StatusCodeUnProcessableEntity int = 422

	// 429 too many request - 请求过多。
	StatusCodeTooManyRequest int = 429

	// 500   内部错误  系统内部错误，例如未知错误。例如kubernetes服务暂停服务（业务错误）。
	StatusCodeServiceError int = 500

	// 800   license is invalid
	StatusCodeLicenseInvalid int = 800
	// 801   license is overload
	StatusCodeLicenseOverload int = 801
)
