package console

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/harvester/harvester/pkg/installer/config"
)

func TestParseWebhook(t *testing.T) {
	tests := []struct {
		name          string
		unparsed      config.Webhook
		context       map[string]string
		parsedURL     string
		parsedPayload string
		errorString   string
	}{
		{
			name: "valid",
			unparsed: config.Webhook{
				Event:  "SUCCEEDED",
				Method: "get",
				Headers: map[string][]string{
					"Content-Type": {"application/json; charset=UTF-8"},
				},
				URL:     "http://10.100.0.10/cblr/svc/op/nopxe/system/{{.Hostname}}/{{.Unknown}}",
				Payload: `{"hostname": "{{.Hostname}}"}`,
			},
			context: map[string]string{
				"Hostname": "node1",
			},
			parsedURL:     "http://10.100.0.10/cblr/svc/op/nopxe/system/node1/",
			parsedPayload: `{"hostname": "node1"}`,
		},
		{
			name: "invalid event",
			unparsed: config.Webhook{
				Event:  "XXX",
				Method: "GET",
				URL:    "http://somewhere.com",
			},
			errorString: "unknown install event: XXX",
		},
		{
			name: "invalid HTTP method",
			unparsed: config.Webhook{
				Event:  "STARTED",
				Method: "PUNCH",
				URL:    "http://somewhere.com",
			},
			errorString: "unknown HTTP method: PUNCH",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := prepareWebhook(tt.unparsed, tt.context)
			if tt.errorString != "" {
				assert.EqualError(t, err, tt.errorString)
			} else {
				assert.Equal(t, tt.parsedURL, p.RenderedURL)
				assert.Equal(t, tt.parsedPayload, p.RenderedPayload)
				assert.Equal(t, nil, err)
			}
		})
	}
}

func TestParsedWebhook_Send(t *testing.T) {
	type fields struct {
		TLS             bool
		Webhook         config.Webhook
		RenderedURL     string
		RenderedPayload string
	}
	type RequestRecorder struct {
		Method  string
		Body    string
		Headers map[string][]string
		Handled bool
	}

	tests := []struct {
		name        string
		fields      fields
		wantMethod  string
		wantHeaders map[string][]string
		wantBody    string
	}{
		{
			name: "get a url",
			fields: fields{
				Webhook: config.Webhook{Method: "GET"},
			},
			wantMethod: "GET",
		},
		{
			name: "get a url from a TLS server",
			fields: fields{
				TLS:     true,
				Webhook: config.Webhook{Method: "GET", Insecure: true},
			},
			wantMethod: "GET",
		},
		{
			name: "get a url with basic auth",
			fields: fields{
				Webhook: config.Webhook{
					Method: "GET",
					BasicAuth: config.HTTPBasicAuth{
						User:     "aaa",
						Password: "bbb",
					},
				},
			},
			wantMethod: "GET",
			wantHeaders: map[string][]string{
				"Authorization": {"Basic YWFhOmJiYg=="},
			},
		},
		{
			name: "put a body",
			fields: fields{
				Webhook: config.Webhook{
					Method: "PUT",
					Headers: map[string][]string{
						"Content-Type": {"application/json; charset=utf-8"},
						"X-Yyy":        {"a", "b"},
					},
				},
				RenderedPayload: "data",
			},
			wantMethod: "PUT",
			wantHeaders: map[string][]string{
				"Content-Type":   {"application/json; charset=utf-8"},
				"Content-Length": {"4"},
				"X-Yyy":          {"a", "b"},
			},
			wantBody: "data",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := RequestRecorder{}
			newTestServer := httptest.NewServer
			if tt.fields.TLS {
				newTestServer = httptest.NewTLSServer
			}
			ts := newTestServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				recorder.Method = r.Method
				recorder.Headers = dupHeaders(r.Header)
				defer r.Body.Close() //nolint:errcheck
				body, err := io.ReadAll(r.Body)
				if err != nil {
					return
				}
				recorder.Body = string(body)
				recorder.Handled = true
			}))
			defer ts.Close()

			p := &RenderedWebhook{
				Webhook:         tt.fields.Webhook,
				RenderedURL:     ts.URL,
				RenderedPayload: tt.fields.RenderedPayload,
			}
			p.Handle() //nolint:errcheck,gosec
			assert.Equal(t, true, recorder.Handled)
			assert.Equal(t, tt.wantMethod, recorder.Method)
			for k, vv := range tt.wantHeaders {
				assert.Contains(t, recorder.Headers, k)
				for _, v := range vv {
					assert.Contains(t, recorder.Headers[k], v)
				}
			}
			assert.Equal(t, tt.wantBody, recorder.Body)
		})
	}
}
