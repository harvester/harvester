package engineapi

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"

	imclient "github.com/longhorn/longhorn-instance-manager/pkg/client"
	imutil "github.com/longhorn/longhorn-instance-manager/pkg/util"

	"github.com/longhorn/longhorn-manager/datastore"
	"github.com/longhorn/longhorn-manager/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func getLoggerForEngineProxyClient(logger logrus.FieldLogger, im *longhorn.InstanceManager) *logrus.Entry {
	return logger.WithFields(
		logrus.Fields{
			"instanceManager": im.Name,
			"image":           im.Spec.Image,
			"serverIP":        im.Status.IP,
		},
	)
}

func GetCompatibleClient(e *longhorn.Engine, fallBack interface{}, ds *datastore.DataStore, logger logrus.FieldLogger, proxyConnCounter util.Counter) (c EngineClientProxy, err error) {
	if e == nil {
		return nil, errors.Errorf("BUG: failed to get engine client proxy due to missing engine")
	}

	im, err := ds.GetInstanceManagerRO(e.Status.InstanceManagerName)
	if err != nil {
		return nil, err
	}

	if logger == nil {
		logger = logrus.StandardLogger()
	}
	log := getLoggerForEngineProxyClient(logger, im)

	shouldFallBack := false

	if im != nil {
		if err := CheckInstanceManagerProxySupport(im); err != nil {
			log.WithError(err).Trace("Use fallback client")
			shouldFallBack = true
		}
	}

	if shouldFallBack {
		if fallBack == nil {
			return nil, errors.Errorf("missing engine client proxy fallback client")
		}

		if obj, ok := fallBack.(*EngineBinary); ok {
			return obj, nil
		}

		return nil, errors.Errorf("BUG: invalid engine client proxy fallback client: %v", fallBack)
	}

	return NewEngineClientProxy(im, log, proxyConnCounter)
}

func NewEngineClientProxy(im *longhorn.InstanceManager, logger logrus.FieldLogger, proxyConnCounter util.Counter) (c EngineClientProxy, err error) {
	defer func() {
		err = errors.Wrap(err, "failed to get engine client proxy")
	}()

	isInstanceManagerRunning := im.Status.CurrentState == longhorn.InstanceManagerStateRunning
	if !isInstanceManagerRunning {
		err = errors.Errorf("%v instance manager is in %v, not running state", im.Name, im.Status.CurrentState)
		return nil, err
	}

	hasIP := im.Status.IP != ""
	if !hasIP {
		err = errors.Errorf("%v instance manager status IP is missing", im.Name)
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	client, err := imclient.NewProxyClient(ctx, cancel, im.Status.IP, InstanceManagerProxyServiceDefaultPort)
	if err != nil {
		return nil, err
	}

	proxyConnCounter.IncreaseCount()

	return &Proxy{
		logger:           logger,
		grpcClient:       client,
		proxyConnCounter: proxyConnCounter,
	}, nil
}

type Proxy struct {
	logger     logrus.FieldLogger
	grpcClient *imclient.ProxyClient

	proxyConnCounter util.Counter
}

type EngineClientProxy interface {
	EngineClient

	Close()
}

func (p *Proxy) Close() {
	if p.grpcClient == nil {
		p.logger.WithError(errors.New("gRPC client not exist")).Warn("Failed to close engine proxy service client")
		return
	}

	if err := p.grpcClient.Close(); err != nil {
		p.logger.WithError(err).Warn("Failed to close engine client proxy")
	}

	// The only potential returning error from Close() is
	// "grpc: the client connection is closing". This means we should still
	// decrease the connection count.
	p.proxyConnCounter.DecreaseCount()
}

func (p *Proxy) DirectToURL(e *longhorn.Engine) string {
	if e == nil {
		p.logger.Debug("BUG: cannot get engine client proxy re-direct URL with nil engine object")
		return ""
	}

	return imutil.GetURL(e.Status.StorageIP, e.Status.Port)
}

func (p *Proxy) VersionGet(e *longhorn.Engine, clientOnly bool) (version *EngineVersion, err error) {
	recvClientVersion := p.grpcClient.ClientVersionGet()
	clientVersion := (*longhorn.EngineVersionDetails)(&recvClientVersion)

	if clientOnly {
		return &EngineVersion{
			ClientVersion: clientVersion,
		}, nil
	}

	recvServerVersion, err := p.grpcClient.ServerVersionGet(p.DirectToURL(e))
	if err != nil {
		return nil, err
	}

	return &EngineVersion{
		ClientVersion: clientVersion,
		ServerVersion: (*longhorn.EngineVersionDetails)(recvServerVersion),
	}, nil
}
