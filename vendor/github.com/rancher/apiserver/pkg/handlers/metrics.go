package handlers

import (
	"strconv"

	"github.com/rancher/apiserver/pkg/apierror"
	"github.com/rancher/apiserver/pkg/metrics"
	"github.com/rancher/apiserver/pkg/types"
)

func MetricsHandler(successCode string, next func(apiRequest *types.APIRequest) (types.APIObject, error)) func(apiRequest *types.APIRequest) (types.APIObject, error) {
	return func(request *types.APIRequest) (types.APIObject, error) {
		obj, err := next(request)
		if err != nil {
			if apiError, ok := err.(*apierror.APIError); ok {

				metrics.IncTotalResponses(request.Schema.ID, request.Method, strconv.Itoa(apiError.Code.Status))
			}
			return types.APIObject{}, err
		}

		metrics.IncTotalResponses(request.Schema.ID, request.Method, successCode)
		return obj, err
	}
}

func MetricsListHandler(successCode string, next func(apiRequest *types.APIRequest) (types.APIObjectList, error)) func(apiRequest *types.APIRequest) (types.APIObjectList, error) {
	return func(request *types.APIRequest) (types.APIObjectList, error) {
		objList, err := next(request)
		if err != nil {
			if apiError, ok := err.(*apierror.APIError); ok {
				metrics.IncTotalResponses(request.Schema.ID, request.Method, strconv.Itoa(apiError.Code.Status))
			}
			return types.APIObjectList{}, err
		}

		metrics.IncTotalResponses(request.Schema.ID, request.Method, successCode)
		return objList, err
	}
}
