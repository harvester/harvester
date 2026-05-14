package console

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"text/template"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/harvester/harvester/pkg/installer/config"
	"github.com/harvester/harvester/pkg/installer/util"
)

const (
	WebhookMaxRetries = 3
)

type RenderedWebhook struct {
	config.Webhook
	RenderedURL     string
	RenderedPayload string
	Valid           bool
}

type RendererWebhooks []RenderedWebhook

const (
	EventInstallStarted  = "STARTED"
	EventInstallSuceeded = "SUCCEEDED"
	EventInstallFailed   = "FAILED"
	PasswordMask         = "******"
)

func IsValidEvent(event string) bool {
	events := []string{
		EventInstallStarted,
		EventInstallSuceeded,
		EventInstallFailed,
	}
	return util.StringSliceContains(events, event)
}

func IsValidHTTPMethod(method string) bool {
	methods := []string{
		http.MethodGet,
		http.MethodHead,
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
		http.MethodConnect,
		http.MethodOptions,
		http.MethodTrace,
	}
	return util.StringSliceContains(methods, method)
}

func (p *RenderedWebhook) DebugOutput(format string) {
	password := p.BasicAuth.Password
	p.BasicAuth.Password = PasswordMask
	logrus.Debugf(format, p)
	p.BasicAuth.Password = password
}

func (p *RenderedWebhook) Handle() error {
	p.DebugOutput("handle webhook: %+v")

	doHTTPReq := func() error {
		c := http.Client{
			Timeout: defaultHTTPTimeout,
		}

		if p.Webhook.Insecure {
			c.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
				},
			}
		}

		var body io.Reader
		if p.RenderedPayload != "" {
			body = strings.NewReader(p.RenderedPayload)
		}

		req, err := http.NewRequest(p.Webhook.Method, p.RenderedURL, body)
		if err != nil {
			return err
		}

		if p.BasicAuth.User != "" && p.BasicAuth.Password != "" {
			req.SetBasicAuth(p.BasicAuth.User, p.BasicAuth.Password)
		}

		for k, vv := range p.Webhook.Headers {
			for _, v := range vv {
				req.Header.Add(k, v)
			}
		}

		resp, err := c.Do(req)
		if err != nil {
			return err
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 400 {
			return fmt.Errorf("got %d status code from %s", resp.StatusCode, p.RenderedURL)
		}
		return nil
	}

	// retries on timeout
	retries := 0
	var retryErr error
	for retries <= WebhookMaxRetries {
		err := doHTTPReq()
		switch e := err.(type) {
		case *url.Error:
			if e.Timeout() {
				time.Sleep(5 * time.Second)
				retries++
				retryErr = err
				continue
			}
		}
		return err
	}
	return retryErr
}

func dupHeaders(h map[string][]string) map[string][]string {
	if h == nil {
		return nil
	}
	m := make(map[string][]string)
	for k, v := range h {
		m[k] = util.DupStrings(v)
	}
	return m
}

func prepareWebhook(h config.Webhook, context map[string]string) (*RenderedWebhook, error) {
	p := &RenderedWebhook{
		Webhook: config.Webhook{
			Event:    h.Event,
			Method:   strings.ToUpper(h.Method),
			Headers:  dupHeaders(h.Headers),
			URL:      h.URL,
			Payload:  h.Payload,
			Insecure: h.Insecure,
			BasicAuth: config.HTTPBasicAuth{
				User:     h.BasicAuth.User,
				Password: h.BasicAuth.Password,
			},
		},
	}

	if !IsValidEvent(p.Webhook.Event) {
		return nil, errors.Errorf("unknown install event: %s", p.Webhook.Event)
	}
	if !IsValidHTTPMethod(p.Webhook.Method) {
		return nil, errors.Errorf("unknown HTTP method: %s", p.Webhook.Method)
	}

	// render URL
	tmplOption := "missingkey=zero"
	bs := bytes.NewBufferString("")
	tmpl, err := template.New("URL").Option(tmplOption).Parse(p.Webhook.URL)
	if err != nil {
		return nil, err
	}
	err = tmpl.Execute(bs, context)
	if err != nil {
		return nil, err
	}
	p.RenderedURL = bs.String()

	// render payload
	bs.Reset()
	tmpl, err = template.New("Payload").Option(tmplOption).Parse(p.Webhook.Payload)
	if err != nil {
		return nil, err
	}
	err = tmpl.Execute(bs, context)
	if err != nil {
		return nil, err
	}
	p.RenderedPayload = bs.String()

	return p, nil
}

func PrepareWebhooks(hooks []config.Webhook, context map[string]string) (RendererWebhooks, error) {
	result := make(RendererWebhooks, 0, len(hooks))
	for _, h := range hooks {
		logrus.Debugf("preparing webhook %+v", h)
		p, err := prepareWebhook(h, context)
		if err != nil {
			msg := fmt.Sprintf("fail to prepare webhook %+v: %s", h, err)
			logrus.Error(msg)
			return nil, errors.New(msg)
		}
		p.Valid = true

		p.DebugOutput("rendered webhook %+v")
		result = append(result, *p)
	}
	return result, nil
}

func (hooks RendererWebhooks) Handle(event string) {
	logrus.Infof("handle webhooks for event %s", event)
	for _, h := range hooks {
		if event != h.Webhook.Event {
			continue
		}
		if err := h.Handle(); err != nil {
			logrus.Errorf("fail to handle webhook: %s", err)
			return
		}
	}
}

func getIPAddr(iface *net.Interface, v6 bool) string {
	addrs, err := iface.Addrs()
	if err == nil {
		for _, addr := range addrs {
			var ip net.IP
			switch t := addr.(type) {
			case *net.IPNet:
				ip = t.IP
			case *net.IPAddr:
				ip = t.IP
			}

			// looks like IPv4 address is represented as IPv6 by default
			if ip != nil && len(ip) == net.IPv6len {
				r := ip.To4()
				if r != nil && !v6 {
					return r.String()
				}
				if r == nil && v6 {
					return ip.String()
				}
			}
		}
	}
	return ""
}

func getWebhookContext(cfg *config.HarvesterConfig) map[string]string {
	// Hostname
	m := map[string]string{
		"Hostname": cfg.Hostname,
	}

	// MAC address and IP addresses
	if iface, err := net.InterfaceByName(config.MgmtInterfaceName); err == nil {
		m["MACAddr"] = iface.HardwareAddr.String()
		m["IPAddrV4"] = getIPAddr(iface, false)
		m["IPAddrV6"] = getIPAddr(iface, true)
	}
	logrus.Debugf("webhook context %+v", m)
	return m
}
