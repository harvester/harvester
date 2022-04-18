package engineapi

import (
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

func NewInstanceManagerClient(im *longhorn.InstanceManager) (*InstanceManagerClient, error) {
	// Do not check the major version here. Since IM cannot get the major version without using this client to call VersionGet().
	if im.Status.CurrentState != longhorn.InstanceManagerStateRunning || im.Status.IP == "" {
		return nil, fmt.Errorf("invalid Instance Manager %v, state: %v, IP: %v", im.Name, im.Status.CurrentState, im.Status.IP)
	}

	return &InstanceManagerClient{
		ip:            im.Status.IP,
		apiMinVersion: im.Status.APIMinVersion,
		apiVersion:    im.Status.APIVersion,
		grpcClient:    imclient.NewProcessManagerClient(imutil.GetURL(im.Status.IP, InstanceManagerDefaultPort)),
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

func (c *InstanceManagerClient) ProcessLog(name string) (*imapi.LogStream, error) {
	if err := CheckInstanceManagerCompatibilty(c.apiMinVersion, c.apiVersion); err != nil {
		return nil, err
	}
	return c.grpcClient.ProcessLog(name)
}

func (c *InstanceManagerClient) ProcessWatch() (*imapi.ProcessStream, error) {
	if err := CheckInstanceManagerCompatibilty(c.apiMinVersion, c.apiVersion); err != nil {
		return nil, err
	}
	return c.grpcClient.ProcessWatch()
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

func (c *InstanceManagerClient) VersionGet() (int, int, error) {
	output, err := c.grpcClient.VersionGet()
	if err != nil {
		return 0, 0, err
	}
	return output.InstanceManagerAPIMinVersion, output.InstanceManagerAPIVersion, nil
}
