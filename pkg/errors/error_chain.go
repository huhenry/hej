package errors

import (
	"errors"
)

type ErrorChainProcess func(err error) (*HttpRespError, bool)

var errorChains = []ErrorChainProcess{
	upstreamErrorProcess,
}

func upstreamErrorProcess(err error) (*HttpRespError, bool) {

	var upError *UpstreamError
	if errors.As(err, &upError) {
		httperr := &HttpRespError{
			HTTPStatus: upError.Code,
			Code:       CodeUpStreamErr,
			Err:        err,
			Message:    upError.Msg,
		}

		return httperr, true
	}

	return nil, false
}

/*
func k8sNotFoundProcess(err error) (*HttpRespError, bool) {
	if k8serrors.IsNotFound(err) {
		if statusError, ok := err.(*k8serrors.StatusError); ok {
			name := statusError.ErrStatus.Details.Name
			message := fmt.Sprintf("%s在集群中不存在", name)

			return &HttpRespError{
				HTTPStatus: http.StatusInternalServerError,
				Message:    message,
				Code:       CodeK8sNotFoundErr,
				Err:        err,
			}, true
		}
	}

	return nil, false
}

func k8sAlreadyExistsProcess(err error) (*HttpRespError, bool) {
	if k8serrors.IsAlreadyExists(err) {
		if statusError, ok := err.(*k8serrors.StatusError); ok {
			name := statusError.ErrStatus.Details.Name
			message := fmt.Sprintf("%s在集群中已存在", name)

			return &HttpRespError{
				HTTPStatus: http.StatusInternalServerError,
				Message:    message,
				Code:       CodeK8sAlreadyExistsErr,
				Err:        err,
			}, true
		}
	}

	return nil, false
}
*/

func ProcessErrorChain(err error) (*HttpRespError, bool) {
	for i := range errorChains {
		if httpErr, ok := errorChains[i](err); ok {
			return httpErr, true
		}
	}
	return nil, false

}
