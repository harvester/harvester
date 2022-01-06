package config

type Options struct {
	Namespace       string
	Threadiness     int
	HTTPSListenPort int

	HarvesterControllerUsername string
	ExposePrometheusMetrics     bool
	GarbageCollectionUsername   string
}
