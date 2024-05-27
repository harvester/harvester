package utils

import (
	"net/http"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/sirupsen/logrus"
)

// RequestWithInspection logs the request URL and method before sending the request to Azure API server for processing.
func RequestWithInspection() autorest.PrepareDecorator {
	return func(p autorest.Preparer) autorest.Preparer {
		return autorest.PreparerFunc(func(r *http.Request) (*http.Request, error) {
			logrus.Info("Azure request", logrus.Fields{
				"method":  r.Method,
				"request": r.URL.String(),
			})
			return p.Prepare(r)
		})
	}
}

// ResponseWithInspection logs the response status, request URL, and request ID after receiving the response from Azure API server.
func ResponseWithInspection() autorest.RespondDecorator {
	return func(r autorest.Responder) autorest.Responder {
		return autorest.ResponderFunc(func(resp *http.Response) error {
			logrus.Info("Azure response", logrus.Fields{
				"status":          resp.Status,
				"method":          resp.Request.Method,
				"request":         resp.Request.URL.String(),
				"x-ms-request-id": azure.ExtractRequestID(resp),
			})
			return r.Respond(resp)
		})
	}
}
