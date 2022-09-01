package ui

import (
	"crypto/tls"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	responsewriter "github.com/rancher/apiserver/pkg/middleware"
	"github.com/sirupsen/logrus"

	"github.com/harvester/harvester/pkg/settings"
)

var (
	insecureClient = &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	Vue = newHandler(settings.UIIndex.Get,
		settings.UIPath.Get,
		settings.UISource.Get)
	VueIndex = Vue.IndexFile()
)

func newHandler(
	indexSetting func() string,
	pathSetting func() string,
	offlineSetting func() string) *handler {
	return &handler{
		indexSetting:   indexSetting,
		offlineSetting: offlineSetting,
		pathSetting:    pathSetting,
		middleware: responsewriter.Chain{
			responsewriter.Gzip,
			responsewriter.FrameOptions,
			responsewriter.CacheMiddleware("json", "js", "css"),
		}.Handler,
		indexMiddleware: responsewriter.Chain{
			responsewriter.Gzip,
			responsewriter.NoCache,
			responsewriter.FrameOptions,
			responsewriter.ContentType,
		}.Handler,
	}
}

type handler struct {
	pathSetting     func() string
	indexSetting    func() string
	offlineSetting  func() string
	middleware      func(http.Handler) http.Handler
	indexMiddleware func(http.Handler) http.Handler

	downloadOnce    sync.Once
	downloadSuccess bool
}

func (u *handler) canDownload(url string) bool {
	u.downloadOnce.Do(func() {
		if err := serveIndex(ioutil.Discard, url); err == nil {
			u.downloadSuccess = true
		} else {
			logrus.Errorf("Failed to download %s, falling back to packaged UI", url)
		}
	})
	return u.downloadSuccess
}

func (u *handler) path() (path string, isURL bool) {
	switch u.offlineSetting() {
	case "auto":
		if settings.IsRelease() {
			return u.pathSetting(), false
		}
		if u.canDownload(u.indexSetting()) {
			return u.indexSetting(), true
		}
		return u.pathSetting(), false
	case "bundled":
		return u.pathSetting(), false
	default:
		return u.indexSetting(), true
	}
}

func (u *handler) ServeAsset() http.Handler {
	return u.middleware(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		http.FileServer(http.Dir(u.pathSetting())).ServeHTTP(rw, req)
	}))
}

func (u *handler) IndexFileOnNotFound() http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		req.URL.Path = strings.TrimPrefix(req.URL.Path, "/dashboard")
		if _, err := os.Stat(filepath.Join(u.pathSetting(), req.URL.Path)); err == nil {
			u.ServeAsset().ServeHTTP(rw, req)
		} else {
			u.IndexFile().ServeHTTP(rw, req)
		}
	})
}

func (u *handler) IndexFile() http.Handler {
	return u.indexMiddleware(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if path, isURL := u.path(); isURL {
			_ = serveIndex(rw, path)
		} else {
			http.ServeFile(rw, req, filepath.Join(path, "index.html"))
		}
	}))
}

func serveIndex(resp io.Writer, url string) error {
	r, err := insecureClient.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	_, err = io.Copy(resp, r.Body)
	return err
}

func (u *handler) PluginServeAsset() http.Handler {
	return u.middleware(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		http.FileServer(http.Dir(u.pathSetting())).ServeHTTP(rw, req)
	}))
}
