package engineapi

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"

	imapi "github.com/longhorn/longhorn-instance-manager/pkg/api"
	imclient "github.com/longhorn/longhorn-instance-manager/pkg/client"
	imutil "github.com/longhorn/longhorn-instance-manager/pkg/util"

	"github.com/longhorn/longhorn-manager/types"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

const (
	CurrentInstanceManagerAPIVersion = 1
	UnknownInstanceManagerAPIVersion = 0

	CurrentInstanceManagerProxyAPIVersion = 1
	UnknownInstanceManagerProxyAPIVersion = 0
	// UnsupportInstanceManagerProxyAPIVersion means the instance manager without the proxy client (Longhorn release before v1.3.0)
	UnsupportInstanceManagerProxyAPIVersion = 0

	DefaultEnginePortCount  = 1
	DefaultReplicaPortCount = 15

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

	// The gRPC client supports backward compatibility.
	grpcClient *imclient.ProcessManagerClient
}

func (c *InstanceManagerClient) Close() error {
	if c.grpcClient == nil {
		return nil
	}

	return c.grpcClient.Close()
}

func GetDeprecatedInstanceManagerBinary(image string) string {
	cname := types.GetImageCanonicalName(image)
	return filepath.Join(types.EngineBinaryDirectoryOnHost, cname, DeprecatedInstanceManagerBinaryName)
}

func CheckInstanceManagerCompatibilty(imMinVersion, imVersion int) error {
	if CurrentInstanceManagerAPIVersion > imVersion || CurrentInstanceManagerAPIVersion < imMinVersion {
		return fmt.Errorf("current InstanceManager version %v is not compatible with InstanceManagerAPIVersion %v and InstanceManagerAPIMinVersion %v",
			CurrentInstanceManagerAPIVersion, imVersion, imMinVersion)
	}
	return nil
}

func CheckInstanceManagerProxyCompatibility(im *longhorn.InstanceManager) error {
	if CurrentInstanceManagerProxyAPIVersion > im.Status.ProxyAPIVersion ||
		CurrentInstanceManagerProxyAPIVersion < im.Status.ProxyAPIMinVersion {
		return fmt.Errorf("current InstanceManager proxy version %v is not compatible with InstanceManagerProxyAPIVersion %v and InstanceManagerProxyAPIMinVersion %v",
			CurrentInstanceManagerAPIVersion, im.Status.ProxyAPIVersion, im.Status.ProxyAPIMinVersion)
	}
	return nil
}

func CheckInstanceManagerProxySupport(im *longhorn.InstanceManager) error {
	if UnsupportInstanceManagerProxyAPIVersion == im.Status.ProxyAPIVersion {
		return fmt.Errorf("%v does not support proxy", im.Name)
	}
	return nil
}

func NewInstanceManagerClient(im *longhorn.InstanceManager) (*InstanceManagerClient, error) {
	// Do not check the major version here. Since IM cannot get the major version without using this client to call VersionGet().
	if im.Status.CurrentState != longhorn.InstanceManagerStateRunning || im.Status.IP == "" {
		return nil, fmt.Errorf("invalid Instance Manager %v, state: %v, IP: %v", im.Name, im.Status.CurrentState, im.Status.IP)
	}
	// HACK: TODO: fix me
	endpoint := "tcp://" + imutil.GetURL(im.Status.IP, InstanceManagerDefaultPort)

	initTLSClient := func() (*imclient.ProcessManagerClient, error) {
		// check for tls cert file presence
		pmClient, err := imclient.NewProcessManagerClientWithTLS(endpoint,
			filepath.Join(types.TLSDirectoryInContainer, types.TLSCAFile),
			filepath.Join(types.TLSDirectoryInContainer, types.TLSCertFile),
			filepath.Join(types.TLSDirectoryInContainer, types.TLSKeyFile),
			"longhorn-backend.longhorn-system",
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load Instance Manager Client TLS files Error: %w", err)
		}

		if _, err = pmClient.VersionGet(); err != nil {
			return nil, fmt.Errorf("failed to check check version of Instance Manager Client with TLS connection Error: %w", err)
		}

		return pmClient, nil
	}

	pmClient, err := initTLSClient()
	if err != nil {
		// fallback to non tls client, there is no way to differentiate between im versions unless we get the version via the im client
		// TODO: remove this im client fallback mechanism in a future version maybe 2.4 / 2.5 or the next time we update the api version
		pmClient, err = imclient.NewProcessManagerClient(endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Instance Manager Client for %v, state: %v, IP: %v, TLS: %v, Error: %w",
				im.Name, im.Status.CurrentState, im.Status.IP, false, err)
		}

		if _, err = pmClient.VersionGet(); err != nil {
			return nil, fmt.Errorf("failed to get Version of Instance Manager Client for %v, state: %v, IP: %v, TLS: %v, Error: %w",
				im.Name, im.Status.CurrentState, im.Status.IP, false, err)
		}
	}

	// TODO: consider evaluating im client version since we do the call anyway to validate the connection, i.e. fallback to non tls
	//  This way we don't need the per call compatibility check, ref: `CheckInstanceManagerCompatibilty`

	return &InstanceManagerClient{
		ip:            im.Status.IP,
		apiMinVersion: im.Status.APIMinVersion,
		apiVersion:    im.Status.APIVersion,
		grpcClient:    pmClient,
	}, nil
}

func (c *InstanceManagerClient) parseProcess(p *imapi.Process) *longhorn.InstanceProcess {
	if p == nil {
		return nil
	}

	return &longhorn.InstanceProcess{
		Spec: longhorn.InstanceProcessSpec{
			Name: p.Name,
		},
		Status: longhorn.InstanceProcessStatus{
			State:     longhorn.InstanceState(p.ProcessStatus.State),
			ErrorMsg:  p.ProcessStatus.ErrorMsg,
			PortStart: p.ProcessStatus.PortStart,
			PortEnd:   p.ProcessStatus.PortEnd,

			// These fields are not used, maybe we can deprecate them later.
			Type:     "",
			Listen:   "",
			Endpoint: "",
		},
	}

}

func (c *InstanceManagerClient) EngineProcessCreate(engineName, volumeName, engineImage string, volumeFrontend longhorn.VolumeFrontend, replicaAddressMap map[string]string, revCounterDisabled bool, salvageRequested bool) (*longhorn.InstanceProcess, error) {
	if err := CheckInstanceManagerCompatibilty(c.apiMinVersion, c.apiVersion); err != nil {
		return nil, err
	}
	frontend, err := GetEngineProcessFrontend(volumeFrontend)
	if err != nil {
		return nil, err
	}
	args := []string{"controller", volumeName, "--frontend", frontend}

	if revCounterDisabled {
		args = append(args, "--disableRevCounter")
	}

	if salvageRequested {
		args = append(args, "--salvageRequested")
	}

	for _, addr := range replicaAddressMap {
		args = append(args, "--replica", GetBackendReplicaURL(addr))
	}
	binary := filepath.Join(types.GetEngineBinaryDirectoryForEngineManagerContainer(engineImage), types.EngineBinaryName)

	engineProcess, err := c.grpcClient.ProcessCreate(
		engineName, binary, DefaultEnginePortCount, args, []string{DefaultPortArg})
	if err != nil {
		return nil, err
	}
	return c.parseProcess(engineProcess), nil
}

func (c *InstanceManagerClient) ReplicaProcessCreate(replicaName, engineImage, dataPath, backingImagePath string, size int64, revCounterDisabled bool) (*longhorn.InstanceProcess, error) {
	if err := CheckInstanceManagerCompatibilty(c.apiMinVersion, c.apiVersion); err != nil {
		return nil, err
	}
	args := []string{
		"replica", types.GetReplicaMountedDataPath(dataPath),
		"--size", strconv.FormatInt(size, 10),
	}
	if backingImagePath != "" {
		args = append(args, "--backing-file", backingImagePath)
	}
	if revCounterDisabled {
		args = append(args, "--disableRevCounter")
	}

	binary := filepath.Join(types.GetEngineBinaryDirectoryForReplicaManagerContainer(engineImage), types.EngineBinaryName)

	replicaProcess, err := c.grpcClient.ProcessCreate(
		replicaName, binary, DefaultReplicaPortCount, args, []string{DefaultPortArg})
	if err != nil {
		return nil, err
	}
	return c.parseProcess(replicaProcess), nil
}

func (c *InstanceManagerClient) ProcessDelete(name string) error {
	if _, err := c.grpcClient.ProcessDelete(name); err != nil {
		return err
	}
	return nil
}

func (c *InstanceManagerClient) ProcessGet(name string) (*longhorn.InstanceProcess, error) {
	if err := CheckInstanceManagerCompatibilty(c.apiMinVersion, c.apiVersion); err != nil {
		return nil, err
	}
	process, err := c.grpcClient.ProcessGet(name)
	if err != nil {
		return nil, err
	}
	return c.parseProcess(process), nil
}

// ProcessLog returns a grpc stream that will be closed when the passed context is cancelled or the underlying grpc client is closed
func (c *InstanceManagerClient) ProcessLog(ctx context.Context, name string) (*imapi.LogStream, error) {
	if err := CheckInstanceManagerCompatibilty(c.apiMinVersion, c.apiVersion); err != nil {
		return nil, err
	}
	return c.grpcClient.ProcessLog(ctx, name)
}

// ProcessWatch returns a grpc stream that will be closed when the passed context is cancelled or the underlying grpc client is closed
func (c *InstanceManagerClient) ProcessWatch(ctx context.Context) (*imapi.ProcessStream, error) {
	if err := CheckInstanceManagerCompatibilty(c.apiMinVersion, c.apiVersion); err != nil {
		return nil, err
	}
	return c.grpcClient.ProcessWatch(ctx)
}

func (c *InstanceManagerClient) ProcessList() (map[string]longhorn.InstanceProcess, error) {
	if err := CheckInstanceManagerCompatibilty(c.apiMinVersion, c.apiVersion); err != nil {
		return nil, err
	}
	processes, err := c.grpcClient.ProcessList()
	if err != nil {
		return nil, err
	}
	result := map[string]longhorn.InstanceProcess{}
	for name, process := range processes {
		result[name] = *c.parseProcess(process)
	}
	return result, nil
}

func (c *InstanceManagerClient) EngineProcessUpgrade(engineName, volumeName, engineImage string, volumeFrontend longhorn.VolumeFrontend, replicaAddressMap map[string]string) (*longhorn.InstanceProcess, error) {
	if err := CheckInstanceManagerCompatibilty(c.apiMinVersion, c.apiVersion); err != nil {
		return nil, err
	}
	frontend, err := GetEngineProcessFrontend(volumeFrontend)
	if err != nil {
		return nil, err
	}
	args := []string{"controller", volumeName, "--frontend", frontend, "--upgrade"}
	for _, addr := range replicaAddressMap {
		args = append(args, "--replica", GetBackendReplicaURL(addr))
	}
	binary := filepath.Join(types.GetEngineBinaryDirectoryForEngineManagerContainer(engineImage), types.EngineBinaryName)

	engineProcess, err := c.grpcClient.ProcessReplace(
		engineName, binary, DefaultEnginePortCount, args, []string{DefaultPortArg}, DefaultTerminateSignal)
	if err != nil {
		return nil, err
	}
	return c.parseProcess(engineProcess), nil
}

func (c *InstanceManagerClient) VersionGet() (int, int, int, int, error) {
	output, err := c.grpcClient.VersionGet()
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return output.InstanceManagerAPIMinVersion, output.InstanceManagerAPIVersion,
		output.InstanceManagerProxyAPIMinVersion, output.InstanceManagerProxyAPIVersion, nil
}
