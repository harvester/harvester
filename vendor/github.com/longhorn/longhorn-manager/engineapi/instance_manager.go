package engineapi

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.uber.org/multierr"

	imapi "github.com/longhorn/longhorn-instance-manager/pkg/api"
	imclient "github.com/longhorn/longhorn-instance-manager/pkg/client"
	immeta "github.com/longhorn/longhorn-instance-manager/pkg/meta"
	imutil "github.com/longhorn/longhorn-instance-manager/pkg/util"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	"github.com/longhorn/longhorn-manager/types"
)

const (
	CurrentInstanceManagerAPIVersion = 5
	MinInstanceManagerAPIVersion     = 1
	UnknownInstanceManagerAPIVersion = 0

	UnknownInstanceManagerProxyAPIVersion = 0
	// UnsupportedInstanceManagerProxyAPIVersion means the instance manager without the proxy client (Longhorn release before v1.3.0)
	UnsupportedInstanceManagerProxyAPIVersion = 0

	DefaultEnginePortCount = 1

	DefaultReplicaPortCountV1 = 10
	DefaultReplicaPortCountV2 = 5

	DefaultPortArg         = "--listen,0.0.0.0:"
	DefaultTerminateSignal = "SIGHUP"

	// IncompatibleInstanceManagerAPIVersion means the instance manager version in v0.7.0
	IncompatibleInstanceManagerAPIVersion = -1
	DeprecatedInstanceManagerBinaryName   = "longhorn-instance-manager"
)

type InstanceManagerClient struct {
	ip            string
	apiMinVersion int
	apiVersion    int

	// TODO: After eliminating all old instance manager pods, this process manager client can be removed.
	// The gRPC client supports backward compatibility.
	instanceServiceGrpcClient *imclient.InstanceServiceClient
	processManagerGrpcClient  *imclient.ProcessManagerClient
	diskServiceGrpcClient     *imclient.DiskServiceClient
}

func (c *InstanceManagerClient) GetAPIVersion() int {
	return c.apiVersion
}

func (c *InstanceManagerClient) Close() error {
	var err error

	if c.processManagerGrpcClient != nil {
		err = multierr.Append(err, c.processManagerGrpcClient.Close())
	}

	if c.diskServiceGrpcClient != nil {
		err = multierr.Append(err, c.diskServiceGrpcClient.Close())
	}

	if c.instanceServiceGrpcClient != nil {
		err = multierr.Append(err, c.instanceServiceGrpcClient.Close())
	}

	return err
}

func GetDeprecatedInstanceManagerBinary(image string) string {
	cname := types.GetImageCanonicalName(image)
	return filepath.Join(types.EngineBinaryDirectoryOnHost, cname, DeprecatedInstanceManagerBinaryName)
}

func CheckInstanceManagerCompatibility(imMinVersion, imVersion int) error {
	if MinInstanceManagerAPIVersion > imVersion || CurrentInstanceManagerAPIVersion < imMinVersion {
		return fmt.Errorf("current InstanceManager version %v-%v is not compatible with InstanceManagerAPIVersion %v and InstanceManagerAPIMinVersion %v",
			CurrentInstanceManagerAPIVersion, MinInstanceManagerAPIVersion, imVersion, imMinVersion)
	}
	return nil
}

func CheckInstanceManagerProxySupport(im *longhorn.InstanceManager) error {
	if UnsupportedInstanceManagerProxyAPIVersion == im.Status.ProxyAPIVersion {
		return fmt.Errorf("%v does not support proxy", im.Name)
	}
	return nil
}

// NewInstanceManagerClient creates a new instance manager client
func NewInstanceManagerClient(im *longhorn.InstanceManager) (*InstanceManagerClient, error) {
	// Do not check the major version here. Since IM cannot get the major version without using this client to call VersionGet().
	if im.Status.CurrentState != longhorn.InstanceManagerStateRunning || im.Status.IP == "" {
		return nil, fmt.Errorf("invalid Instance Manager %v, state: %v, IP: %v", im.Name, im.Status.CurrentState, im.Status.IP)
	}

	// TODO: Initialize the following gRPC clients are similar. This can be simplified via factory method.

	initProcessManagerTLSClient := func(endpoint string) (*imclient.ProcessManagerClient, error) {
		// check for tls cert file presence
		pmClient, err := imclient.NewProcessManagerClientWithTLS(endpoint,
			filepath.Join(types.TLSDirectoryInContainer, types.TLSCAFile),
			filepath.Join(types.TLSDirectoryInContainer, types.TLSCertFile),
			filepath.Join(types.TLSDirectoryInContainer, types.TLSKeyFile),
			"longhorn-backend.longhorn-system",
		)
		if err != nil {
			return nil, errors.Wrap(err, "failed to load Instance Manager Process Manager Service Client TLS files")
		}
		if _, err = pmClient.VersionGet(); err != nil {
			return nil, errors.Wrap(err, "failed to check version of  Instance Manager Process Manager Service Client with TLS connection")
		}

		return pmClient, nil
	}

	initInstanceServiceTLSClient := func(endpoint string) (*imclient.InstanceServiceClient, error) {
		// check for tls cert file presence
		instanceClient, err := imclient.NewInstanceServiceClientWithTLS(endpoint,
			filepath.Join(types.TLSDirectoryInContainer, types.TLSCAFile),
			filepath.Join(types.TLSDirectoryInContainer, types.TLSCertFile),
			filepath.Join(types.TLSDirectoryInContainer, types.TLSKeyFile),
			"longhorn-backend.longhorn-system",
		)
		if err != nil {
			return nil, errors.Wrap(err, "failed to load Instance Manager Instance Service Client TLS files")
		}
		if _, err = instanceClient.VersionGet(); err != nil {
			return nil, errors.Wrap(err, "failed to check version of Instance Manager Instance Service Client with TLS connection")
		}

		return instanceClient, nil
	}

	initDiskServiceTLSClient := func(endpoint string) (*imclient.DiskServiceClient, error) {
		// check for tls cert file presence
		diskClient, err := imclient.NewDiskServiceClientWithTLS(endpoint,
			filepath.Join(types.TLSDirectoryInContainer, types.TLSCAFile),
			filepath.Join(types.TLSDirectoryInContainer, types.TLSCertFile),
			filepath.Join(types.TLSDirectoryInContainer, types.TLSKeyFile),
			"longhorn-backend.longhorn-system",
		)
		if err != nil {
			return nil, errors.Wrap(err, "failed to load Instance Manager Disk Service Client TLS files")
		}
		if _, err = diskClient.VersionGet(); err != nil {
			return nil, errors.Wrap(err, "failed to check version of  Instance Manager Disk Service Client with TLS connection")
		}

		return diskClient, nil
	}

	// Create a new process manager client
	// HACK: TODO: fix me
	endpoint := "tcp://" + imutil.GetURL(im.Status.IP, InstanceManagerProcessManagerServiceDefaultPort)
	processManagerClient, err := initProcessManagerTLSClient(endpoint)
	if err != nil {
		// fallback to non tls client, there is no way to differentiate between im versions unless we get the version via the im client
		// TODO: remove this im client fallback mechanism in a future version maybe 2.4 / 2.5 or the next time we update the api version
		processManagerClient, err = imclient.NewProcessManagerClient(endpoint, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to initialize Instance Manager Process Manager Service Client for %v, state: %v, IP: %v, TLS: %v",
				im.Name, im.Status.CurrentState, im.Status.IP, false)
		}

		version, err := processManagerClient.VersionGet()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get Version of Instance Manager Process Manager Service Client for %v, state: %v, IP: %v, TLS: %v",
				im.Name, im.Status.CurrentState, im.Status.IP, false)
		}
		logrus.Tracef("Instance Manager Process Manager Service Client Version: %+v", version)
	}

	if im.Status.APIVersion < 4 {
		return &InstanceManagerClient{
			ip:                       im.Status.IP,
			apiMinVersion:            im.Status.APIMinVersion,
			apiVersion:               im.Status.APIVersion,
			processManagerGrpcClient: processManagerClient,
		}, nil
	}

	// Create a new instance service  client
	endpoint = "tcp://" + imutil.GetURL(im.Status.IP, InstanceManagerInstanceServiceDefaultPort)
	instanceServiceClient, err := initInstanceServiceTLSClient(endpoint)
	if err != nil {
		// fallback to non tls client, there is no way to differentiate between im versions unless we get the version via the im client
		// TODO: remove this im client fallback mechanism in a future version maybe 2.4 / 2.5 or the next time we update the api version
		instanceServiceClient, err = imclient.NewInstanceServiceClient(endpoint, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to initialize Instance Manager Instance Service Client for %v, state: %v, IP: %v, TLS: %v",
				im.Name, im.Status.CurrentState, im.Status.IP, false)
		}

		version, err := instanceServiceClient.VersionGet()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get Version of Instance Manager Instance Service Client for %v, state: %v, IP: %v, TLS: %v",
				im.Name, im.Status.CurrentState, im.Status.IP, false)
		}
		logrus.Tracef("Instance Manager Instance Service Client Version: %+v", version)
	}

	// Create a new disk service client
	endpoint = "tcp://" + imutil.GetURL(im.Status.IP, InstanceManagerDiskServiceDefaultPort)
	diskServiceClient, err := initDiskServiceTLSClient(endpoint)
	if err != nil {
		diskServiceClient, err = imclient.NewDiskServiceClient(endpoint, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to initialize Instance Manager Disk Service Client for %v, state: %v, IP: %v, TLS: %v",
				im.Name, im.Status.CurrentState, im.Status.IP, false)
		}
		version, err := diskServiceClient.VersionGet()
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get Version of Instance Manager Disk Service Client for %v, state: %v, IP: %v, TLS: %v",
				im.Name, im.Status.CurrentState, im.Status.IP, false)
		}
		logrus.Tracef("Instance Manager Disk Service Client Version: %+v", version)
	}

	// TODO: consider evaluating im client version since we do the call anyway to validate the connection, i.e. fallback to non tls
	// This way we don't need the per call compatibility check, ref: `CheckInstanceManagerCompatibility`

	return &InstanceManagerClient{
		ip:                        im.Status.IP,
		apiMinVersion:             im.Status.APIMinVersion,
		apiVersion:                im.Status.APIVersion,
		instanceServiceGrpcClient: instanceServiceClient,
		processManagerGrpcClient:  processManagerClient,
		diskServiceGrpcClient:     diskServiceClient,
	}, nil
}

func parseInstance(p *imapi.Instance) *longhorn.InstanceProcess {
	if p == nil {
		return nil
	}

	return &longhorn.InstanceProcess{
		Spec: longhorn.InstanceProcessSpec{
			Name:               p.Name,
			BackendStoreDriver: longhorn.BackendStoreDriverType(p.BackendStoreDriver),
		},
		Status: longhorn.InstanceProcessStatus{
			Type:      getTypeForInstance(longhorn.InstanceType(p.Type), p.PortCount),
			State:     longhorn.InstanceState(p.InstanceStatus.State),
			ErrorMsg:  p.InstanceStatus.ErrorMsg,
			PortStart: p.InstanceStatus.PortStart,
			PortEnd:   p.InstanceStatus.PortEnd,

			// These fields are not used, maybe we can deprecate them later.
			Listen:   "",
			Endpoint: "",
		},
	}
}

func parseProcess(p *imapi.Process) *longhorn.InstanceProcess {
	if p == nil {
		return nil
	}

	return &longhorn.InstanceProcess{
		Spec: longhorn.InstanceProcessSpec{
			Name:               p.Name,
			BackendStoreDriver: longhorn.BackendStoreDriverTypeV1,
		},
		Status: longhorn.InstanceProcessStatus{
			Type:      getTypeForProcess(p.PortCount),
			State:     longhorn.InstanceState(p.ProcessStatus.State),
			ErrorMsg:  p.ProcessStatus.ErrorMsg,
			PortStart: p.ProcessStatus.PortStart,
			PortEnd:   p.ProcessStatus.PortEnd,
		},
	}
}

func getTypeForInstance(instanceType longhorn.InstanceType, portCount int32) longhorn.InstanceType {
	if instanceType != longhorn.InstanceType("") {
		return instanceType
	}

	if portCount == DefaultEnginePortCount {
		return longhorn.InstanceTypeEngine
	}
	return longhorn.InstanceTypeReplica
}

func getTypeForProcess(portCount int32) longhorn.InstanceType {
	if portCount == DefaultEnginePortCount {
		return longhorn.InstanceTypeEngine
	}
	return longhorn.InstanceTypeReplica
}

func getBinaryAndArgsForEngineProcessCreation(e *longhorn.Engine,
	frontend string, engineReplicaTimeout, replicaFileSyncHTTPClientTimeout int64,
	dataLocality longhorn.DataLocality, engineCLIAPIVersion int) (string, []string, error) {

	args := []string{"controller", e.Spec.VolumeName,
		"--frontend", frontend,
	}

	if e.Spec.RevisionCounterDisabled {
		args = append(args, "--disableRevCounter")
	}

	if e.Spec.SalvageRequested {
		args = append(args, "--salvageRequested")
	}

	if engineCLIAPIVersion >= 6 {
		args = append(args,
			"--size", strconv.FormatInt(e.Spec.VolumeSize, 10),
			"--current-size", strconv.FormatInt(e.Status.CurrentSize, 10))
	}

	if engineCLIAPIVersion >= 7 {
		args = append(args,
			"--engine-replica-timeout", strconv.FormatInt(engineReplicaTimeout, 10),
			"--file-sync-http-client-timeout", strconv.FormatInt(replicaFileSyncHTTPClientTimeout, 10))

		if dataLocality == longhorn.DataLocalityStrictLocal {
			args = append(args, "--data-server-protocol", "unix")
		}

		if e.Spec.UnmapMarkSnapChainRemovedEnabled {
			args = append(args, "--unmap-mark-snap-chain-removed")
		}
	}

	if engineCLIAPIVersion >= 9 {
		args = append([]string{"--engine-instance-name", e.Name}, args...)
	}

	for _, addr := range e.Status.CurrentReplicaAddressMap {
		args = append(args, "--replica", GetBackendReplicaURL(addr))
	}
	binary := filepath.Join(types.GetEngineBinaryDirectoryForEngineManagerContainer(e.Spec.EngineImage), types.EngineBinaryName)

	return binary, args, nil
}

func getBinaryAndArgsForReplicaProcessCreation(r *longhorn.Replica,
	dataPath, backingImagePath string, dataLocality longhorn.DataLocality, portCount, engineCLIAPIVersion int) (string, []string) {

	args := []string{
		"replica", types.GetReplicaMountedDataPath(dataPath),
		"--size", strconv.FormatInt(r.Spec.VolumeSize, 10),
	}
	if backingImagePath != "" {
		args = append(args, "--backing-file", backingImagePath)
	}
	if r.Spec.RevisionCounterDisabled {
		args = append(args, "--disableRevCounter")
	}
	if engineCLIAPIVersion >= 7 {
		if engineCLIAPIVersion < 9 {
			// Replaced by the global --volume-name flag when engineCLIAPIVersion == 9.
			args = append(args, "--volume-name", r.Spec.VolumeName)
		}

		if dataLocality == longhorn.DataLocalityStrictLocal {
			args = append(args, "--data-server-protocol", "unix")
		}

		if r.Spec.UnmapMarkDiskChainRemovedEnabled {
			args = append(args, "--unmap-mark-disk-chain-removed")
		}
	}

	if engineCLIAPIVersion >= 9 {
		args = append(args, "--replica-instance-name", r.Name)
		args = append([]string{"--volume-name", r.Spec.VolumeName}, args...)
	}

	// 3 ports are already used by replica server, data server and syncagent server
	syncAgentPortCount := portCount - 3
	args = append(args, "--sync-agent-port-count", strconv.Itoa(syncAgentPortCount))

	binary := filepath.Join(types.GetEngineBinaryDirectoryForReplicaManagerContainer(r.Spec.EngineImage), types.EngineBinaryName)

	return binary, args
}

type EngineInstanceCreateRequest struct {
	Engine                           *longhorn.Engine
	VolumeFrontend                   longhorn.VolumeFrontend
	EngineReplicaTimeout             int64
	ReplicaFileSyncHTTPClientTimeout int64
	DataLocality                     longhorn.DataLocality
	ImIP                             string
	EngineCLIAPIVersion              int
}

// EngineInstanceCreate creates a new engine instance
func (c *InstanceManagerClient) EngineInstanceCreate(req *EngineInstanceCreateRequest) (*longhorn.InstanceProcess, error) {
	if err := CheckInstanceManagerCompatibility(c.apiMinVersion, c.apiVersion); err != nil {
		return nil, err
	}

	binary := ""
	args := []string{}
	replicaAddresses := map[string]string{}

	var err error

	frontend, err := GetEngineInstanceFrontend(req.Engine.Spec.BackendStoreDriver, req.VolumeFrontend)
	if err != nil {
		return nil, err
	}

	switch req.Engine.Spec.BackendStoreDriver {
	case longhorn.BackendStoreDriverTypeV1:
		binary, args, err = getBinaryAndArgsForEngineProcessCreation(req.Engine, frontend, req.EngineReplicaTimeout, req.ReplicaFileSyncHTTPClientTimeout, req.DataLocality, req.EngineCLIAPIVersion)
		if err != nil {
			return nil, err
		}
	case longhorn.BackendStoreDriverTypeV2:
		replicaAddresses = req.Engine.Status.CurrentReplicaAddressMap
	}

	if c.GetAPIVersion() < 4 {
		/* Fall back to the old way of creating engine process */
		process, err := c.processManagerGrpcClient.ProcessCreate(req.Engine.Name, binary, DefaultEnginePortCount, args, []string{DefaultPortArg})
		if err != nil {
			return nil, err
		}
		return parseProcess(imapi.RPCToProcess(process)), nil
	}

	instance, err := c.instanceServiceGrpcClient.InstanceCreate(&imclient.InstanceCreateRequest{
		BackendStoreDriver: string(req.Engine.Spec.BackendStoreDriver),
		Name:               req.Engine.Name,
		InstanceType:       string(longhorn.InstanceManagerTypeEngine),
		VolumeName:         req.Engine.Spec.VolumeName,
		Size:               uint64(req.Engine.Spec.VolumeSize),
		PortCount:          DefaultEnginePortCount,
		PortArgs:           []string{DefaultPortArg},

		Binary:     binary,
		BinaryArgs: args,

		Engine: imclient.EngineCreateRequest{
			ReplicaAddressMap: replicaAddresses,
			Frontend:          frontend,
		},
	})

	if err != nil {
		return nil, err
	}
	return parseInstance(instance), nil
}

type ReplicaInstanceCreateRequest struct {
	Replica             *longhorn.Replica
	DiskName            string
	DataPath            string
	BackingImagePath    string
	DataLocality        longhorn.DataLocality
	ExposeRequired      bool
	ImIP                string
	EngineCLIAPIVersion int
}

// ReplicaInstanceCreate creates a new replica instance
func (c *InstanceManagerClient) ReplicaInstanceCreate(req *ReplicaInstanceCreateRequest) (*longhorn.InstanceProcess, error) {
	if err := CheckInstanceManagerCompatibility(c.apiMinVersion, c.apiVersion); err != nil {
		return nil, err
	}

	binary := ""
	args := []string{}
	if req.Replica.Spec.BackendStoreDriver == longhorn.BackendStoreDriverTypeV1 {
		binary, args = getBinaryAndArgsForReplicaProcessCreation(req.Replica, req.DataPath, req.BackingImagePath, req.DataLocality, DefaultReplicaPortCountV1, req.EngineCLIAPIVersion)
	}

	if c.GetAPIVersion() < 4 {
		/* Fall back to the old way of creating replica process */
		process, err := c.processManagerGrpcClient.ProcessCreate(req.Replica.Name, binary, DefaultReplicaPortCountV1, args, []string{DefaultPortArg})
		if err != nil {
			return nil, err
		}
		return parseProcess(imapi.RPCToProcess(process)), nil
	}

	portCount := DefaultReplicaPortCountV1
	if req.Replica.Spec.BackendStoreDriver == longhorn.BackendStoreDriverTypeV2 {
		portCount = DefaultReplicaPortCountV2
	}

	instance, err := c.instanceServiceGrpcClient.InstanceCreate(&imclient.InstanceCreateRequest{
		BackendStoreDriver: string(req.Replica.Spec.BackendStoreDriver),
		Name:               req.Replica.Name,
		InstanceType:       string(longhorn.InstanceManagerTypeReplica),
		VolumeName:         req.Replica.Spec.VolumeName,
		Size:               uint64(req.Replica.Spec.VolumeSize),
		PortCount:          portCount,
		PortArgs:           []string{DefaultPortArg},

		Binary:     binary,
		BinaryArgs: args,

		Replica: imclient.ReplicaCreateRequest{
			DiskName:       req.DiskName,
			DiskUUID:       req.Replica.Spec.DiskID,
			ExposeRequired: req.ExposeRequired,
		},
	})
	if err != nil {
		return nil, err
	}
	return parseInstance(instance), nil
}

// InstanceDelete deletes the instance
func (c *InstanceManagerClient) InstanceDelete(backendStoreDriver longhorn.BackendStoreDriverType, name, kind, diskUUID string, cleanupRequired bool) (err error) {
	if c.GetAPIVersion() < 4 {
		/* Fall back to the old way of deleting process */
		_, err = c.processManagerGrpcClient.ProcessDelete(name)
	} else {
		_, err = c.instanceServiceGrpcClient.InstanceDelete(string(backendStoreDriver), name, kind, diskUUID, cleanupRequired)
	}

	return err
}

// InstanceGet returns the instance process
func (c *InstanceManagerClient) InstanceGet(backendStoreDriver longhorn.BackendStoreDriverType, name, kind string) (*longhorn.InstanceProcess, error) {
	if err := CheckInstanceManagerCompatibility(c.apiMinVersion, c.apiVersion); err != nil {
		return nil, err
	}

	if c.GetAPIVersion() < 4 {
		/* Fall back to the old way of getting process */
		process, err := c.processManagerGrpcClient.ProcessGet(name)
		if err != nil {
			return nil, err
		}
		return parseProcess(imapi.RPCToProcess(process)), nil
	}

	instance, err := c.instanceServiceGrpcClient.InstanceGet(string(backendStoreDriver), name, kind)
	if err != nil {
		return nil, err
	}
	return parseInstance(instance), nil
}

// InstanceGetBinary returns the binary name of the instance
func (c *InstanceManagerClient) InstanceGetBinary(backendStoreDriver longhorn.BackendStoreDriverType, name, kind, diskUUID string) (string, error) {
	if err := CheckInstanceManagerCompatibility(c.apiMinVersion, c.apiVersion); err != nil {
		return "", err
	}

	if c.GetAPIVersion() < 4 {
		/* Fall back to the old way of getting binary name */
		process, err := c.processManagerGrpcClient.ProcessGet(name)
		if err != nil {
			return "", err
		}
		return imapi.RPCToProcess(process).Binary, nil
	}

	instance, err := c.instanceServiceGrpcClient.InstanceGet(string(backendStoreDriver), name, kind)
	if err != nil {
		return "", err
	}

	if instance.InstanceProccessSpec == nil {
		return "", fmt.Errorf("instance %v has no InstanceProccessSpec", name)
	}
	return instance.InstanceProccessSpec.Binary, nil
}

// InstanceLog returns a grpc stream that will be closed when the passed context is cancelled or the underlying grpc client is closed
func (c *InstanceManagerClient) InstanceLog(ctx context.Context, backendStoreDriver longhorn.BackendStoreDriverType, name, kind string) (*imapi.LogStream, error) {
	if err := CheckInstanceManagerCompatibility(c.apiMinVersion, c.apiVersion); err != nil {
		return nil, err
	}

	if c.GetAPIVersion() < 4 {
		/* Fall back to the old way of logging process */
		return c.processManagerGrpcClient.ProcessLog(ctx, name)
	}

	return c.instanceServiceGrpcClient.InstanceLog(ctx, string(backendStoreDriver), name, kind)
}

// InstanceWatch returns a grpc stream that will be closed when the passed context is cancelled or the underlying grpc client is closed
func (c *InstanceManagerClient) InstanceWatch(ctx context.Context) (interface{}, error) {
	if err := CheckInstanceManagerCompatibility(c.apiMinVersion, c.apiVersion); err != nil {
		return nil, err
	}

	if c.GetAPIVersion() < 4 {
		/* Fall back to the old way of creating replica process */
		return c.processManagerGrpcClient.ProcessWatch(ctx)
	}

	return c.instanceServiceGrpcClient.InstanceWatch(ctx)
}

// InstanceList returns a map of instance name to instance process
func (c *InstanceManagerClient) InstanceList() (map[string]longhorn.InstanceProcess, error) {
	if err := CheckInstanceManagerCompatibility(c.apiMinVersion, c.apiVersion); err != nil {
		return nil, err
	}

	result := map[string]longhorn.InstanceProcess{}

	if c.GetAPIVersion() < 4 {
		/* Fall back to the old way of listing processes */
		processes, err := c.processManagerGrpcClient.ProcessList()
		if err != nil {
			return nil, err
		}
		result := map[string]longhorn.InstanceProcess{}
		for name, process := range processes {
			result[name] = *parseProcess(imapi.RPCToProcess(process))
		}
		return result, nil
	}

	instances, err := c.instanceServiceGrpcClient.InstanceList()
	if err != nil {
		return nil, err
	}
	for name, instance := range instances {
		result[name] = *parseInstance(instance)
	}

	return result, nil
}

type EngineInstanceUpgradeRequest struct {
	Engine                           *longhorn.Engine
	VolumeFrontend                   longhorn.VolumeFrontend
	EngineReplicaTimeout             int64
	ReplicaFileSyncHTTPClientTimeout int64
	DataLocality                     longhorn.DataLocality
	EngineCLIAPIVersion              int
}

// EngineInstanceUpgrade upgrades the engine process
func (c *InstanceManagerClient) EngineInstanceUpgrade(req *EngineInstanceUpgradeRequest) (*longhorn.InstanceProcess, error) {
	engine := req.Engine
	switch engine.Spec.BackendStoreDriver {
	case longhorn.BackendStoreDriverTypeV1:
		return c.engineInstanceUpgrade(req)
	case longhorn.BackendStoreDriverTypeV2:
		/* TODO: Handle SPDK engine upgrade */
		return nil, fmt.Errorf("SPDK engine upgrade is not supported yet")
	default:
		return nil, fmt.Errorf("unknown backend store driver %v", engine.Spec.BackendStoreDriver)
	}
}

func (c *InstanceManagerClient) engineInstanceUpgrade(req *EngineInstanceUpgradeRequest) (*longhorn.InstanceProcess, error) {
	if err := CheckInstanceManagerCompatibility(c.apiMinVersion, c.apiVersion); err != nil {
		return nil, err
	}

	frontend, err := GetEngineInstanceFrontend(req.Engine.Spec.BackendStoreDriver, req.VolumeFrontend)
	if err != nil {
		return nil, err
	}
	args := []string{"controller", req.Engine.Spec.VolumeName, "--frontend", frontend, "--upgrade"}
	for _, addr := range req.Engine.Spec.UpgradedReplicaAddressMap {
		args = append(args, "--replica", GetBackendReplicaURL(addr))
	}

	if req.EngineCLIAPIVersion >= 6 {
		args = append(args,
			"--size", strconv.FormatInt(req.Engine.Spec.VolumeSize, 10),
			"--current-size", strconv.FormatInt(req.Engine.Status.CurrentSize, 10))
	}

	if req.EngineCLIAPIVersion >= 7 {
		args = append(args,
			"--engine-replica-timeout", strconv.FormatInt(req.EngineReplicaTimeout, 10),
			"--file-sync-http-client-timeout", strconv.FormatInt(req.ReplicaFileSyncHTTPClientTimeout, 10))

		if req.DataLocality == longhorn.DataLocalityStrictLocal {
			args = append(args,
				"--data-server-protocol", "unix")
		}

		if req.Engine.Spec.UnmapMarkSnapChainRemovedEnabled {
			args = append(args, "--unmap-mark-snap-chain-removed")
		}
	}

	if req.EngineCLIAPIVersion >= 9 {
		args = append([]string{"--engine-instance-name", req.Engine.Name}, args...)
	}

	binary := filepath.Join(types.GetEngineBinaryDirectoryForEngineManagerContainer(req.Engine.Spec.EngineImage), types.EngineBinaryName)

	if c.GetAPIVersion() < 4 {
		process, err := c.processManagerGrpcClient.ProcessReplace(
			req.Engine.Name, binary, DefaultEnginePortCount, args, []string{DefaultPortArg}, DefaultTerminateSignal)
		if err != nil {
			return nil, err
		}
		return parseProcess(imapi.RPCToProcess(process)), nil
	}

	instance, err := c.instanceServiceGrpcClient.InstanceReplace(string(req.Engine.Spec.BackendStoreDriver), req.Engine.Name,
		string(longhorn.InstanceManagerTypeEngine), binary, DefaultEnginePortCount, args, []string{DefaultPortArg}, DefaultTerminateSignal)
	if err != nil {
		return nil, err
	}
	return parseInstance(instance), nil
}

// VersionGet returns the version of the instance manager
func (c *InstanceManagerClient) VersionGet() (int, int, int, int, error) {
	var err error
	var output *immeta.VersionOutput

	if c.GetAPIVersion() < 4 {
		/* Fall back to the old way of getting version */
		output, err = c.processManagerGrpcClient.VersionGet()
	} else {
		output, err = c.instanceServiceGrpcClient.VersionGet()
	}
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return output.InstanceManagerAPIMinVersion, output.InstanceManagerAPIVersion,
		output.InstanceManagerProxyAPIMinVersion, output.InstanceManagerProxyAPIVersion, nil
}
