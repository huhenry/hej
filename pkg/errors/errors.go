package errors

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type ErrorReason int

const (
	CodeParameterRequiredErr = 40001
	CodeBadParametersErr     = 40002
	CodeDynamicClientErr     = 50001
	CodeCustomClientErr      = 50002
	CodeOriginErr            = 50010
	CodeUpStreamErr          = 50003
	CodeK8sNotFoundErr       = 40401
	CodeK8sAlreadyExistsErr  = 30801

	ReasonBadRequest ErrorReason = iota
	ReasonUnauthorized
	ReasonNotFound
	ReasonConflict
	ReasonAlreadyExist
	ReasonInternalException
)

type CustomError interface {
	Error() string
	Reason() ErrorReason
}

type CustomErrorAdapter struct {
	Message     string
	ErrorReason ErrorReason
	Code        int
}

func (adapter *CustomErrorAdapter) Error() string {
	return adapter.Message
}

func (adapter *CustomErrorAdapter) Reason() ErrorReason {
	return adapter.ErrorReason
}

type BadRequestError struct {
	CustomErrorAdapter
}

func BadRequest(message string) error {
	err := &BadRequestError{}
	err.Message = message
	err.ErrorReason = ReasonBadRequest
	err.Code = http.StatusBadRequest

	return err
}

type ConflictError struct {
	CustomErrorAdapter
}

func Conflict(message string) error {
	err := &ConflictError{}
	err.Message = message
	err.ErrorReason = ReasonConflict
	err.Code = http.StatusConflict

	return err
}

type AlreadyExistError struct {
	CustomErrorAdapter
}

func AlreadyExist(message string) error {
	err := &AlreadyExistError{}
	err.Message = message
	err.ErrorReason = ReasonAlreadyExist
	err.Code = http.StatusConflict

	return err
}

// HttpRespError contains custom code, error message, and HTTP status code.
type HttpRespError struct {
	HTTPStatus int
	Code       int
	Message    string
	Err        error
}

func (e *HttpRespError) Error() string {
	return e.Message
}

func (e *HttpRespError) Unwrap() error {
	return e.Err
}

func (e *HttpRespError) IsOriginError() bool {
	return e.Code == CodeOriginErr
}

// WriteToResponse writes response for the error.
func (e *HttpRespError) WriteToResponse(w http.ResponseWriter) {
	w.WriteHeader(e.HTTPStatus)
	fmt.Fprintf(w, e.ToJSON())
	// TODO: store e.ToJSON() to ElasticSearch for future analysis
}

// ToJSON returns JSON string for a MyError.
func (e *HttpRespError) ToJSON() string {
	j, err := json.Marshal(e)
	if err != nil {
		return `{"code":50099,"message":"ScrapError.JSONStr: json.Marshal() failed"}`
	}
	return string(j)
}

// ParameterRequiredErr .
func ParameterRequiredErr(msg string) *HttpRespError {
	return &HttpRespError{
		HTTPStatus: http.StatusBadRequest,
		Code:       CodeParameterRequiredErr,
		Message:    msg,
	}
}

// BadParametersErr .
func BadParametersErr(err error) *HttpRespError {
	return &HttpRespError{
		HTTPStatus: http.StatusBadRequest,
		Code:       CodeBadParametersErr,
		Err:        err,
	}
}

// DynamicClientErr .
func DynamicClientErr(err error) *HttpRespError {

	if httpErr, ok := ProcessErrorChain(err); ok {
		return httpErr
	}
	return &HttpRespError{
		HTTPStatus: http.StatusInternalServerError,
		Code:       CodeDynamicClientErr,
		Err:        err,
	}
}

// CustomClientErr .
func CustomClientErr(msg string, err error) *HttpRespError {

	if httpErr, ok := ProcessErrorChain(err); ok {
		return httpErr
	}
	return &HttpRespError{
		HTTPStatus: http.StatusInternalServerError,
		Code:       CodeCustomClientErr,
		Err:        err,
		Message:    msg,
	}
}

// CustomClientErr .
func OriginErr(err error) *HttpRespError {
	if httpErr, ok := ProcessErrorChain(err); ok {
		return httpErr
	}
	return &HttpRespError{
		Code: CodeOriginErr,
		Err:  err,
	}
}
