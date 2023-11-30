package nfs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"text/template"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	defaultPidFile = "/var/run/ganesha.pid"
)

var defaultConfig = []byte(`
NFS_Core_Param
{
    NLM_Port = 0;
    MNT_Port = 0;
    RQUOTA_Port = 0;
    Enable_NLM = false;
    Enable_RQUOTA = false;
    Enable_UDP = false;   
    fsid_device = false;
    Protocols = 4;
}

LOG {
	Default_Log_Level = INFO;

# 	uncomment to enable debug logging
#	COMPONENTS { NFS_V4 = FULL_DEBUG; }

	Facility {
		name = FILE;
		destination = "{{.LogPath}}";
		enable = active;
	}
}

NFSV4
{
    Lease_Lifetime = 60;
    Grace_Period = 90;
    Minor_Versions = 1, 2;
    RecoveryBackend = longhorn;
    Only_Numeric_Owners = true;
}

Export_defaults
{
    Protocols = 4;
    Transports = TCP;
    Access_Type = None;
    SecType = sys;
    Squash = None;
}

# Pseudo export, ganesha will automatically create one
# if one is not present
#EXPORT
#{
#    Export_Id = 0;
#    Protocols = 4;
#    Transports = TCP;
#    Access_Type = RW;
#    SecType = sys;
#    Squash = None;
#    Path = /export;
#    Pseudo = /;
#    FSAL { Name = VFS; }
#}
`)

type Server struct {
	logger     logrus.FieldLogger
	configPath string
	exportPath string
	exporter   *Exporter
}

func NewServer(logger logrus.FieldLogger, configPath, exportPath, volume string) (*Server, error) {
	if err := setRlimitNOFILE(logger); err != nil {
		logger.WithError(err).Warn("Error setting RLIMIT_NOFILE, there may be 'Too many open files' errors later")
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err = os.WriteFile(configPath, getUpdatedGaneshConfig(defaultConfig), 0600); err != nil {
			return nil, errors.Wrapf(err, "error writing nfs config %s", configPath)
		}
	}

	exporter, err := NewExporter(configPath, exportPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create nfs exporter")
	}

	return &Server{
		logger:     logger,
		configPath: configPath,
		exportPath: exportPath,
		exporter:   exporter,
	}, nil
}

func (s *Server) CreateExport(volume string) (uint16, error) {
	return s.exporter.CreateExport(volume)
}

func (s *Server) Run(ctx context.Context) error {
	// Start ganesha.nfsd
	s.logger.Info("Running NFS server!")
	cmd := exec.CommandContext(ctx, "ganesha.nfsd", "-F", "-p", defaultPidFile, "-f", s.configPath)

	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ganesha.nfsd failed with error: %v, output: %s", err, out)
	}

	return nil
}

func setRlimitNOFILE(logger logrus.FieldLogger) error {
	var rlimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit)
	if err != nil {
		return errors.Wrap(err, "error getting RLIMIT_NOFILE")
	}
	logger.Infof("starting RLIMIT_NOFILE rlimit.Cur %d, rlimit.Max %d", rlimit.Cur, rlimit.Max)
	rlimit.Max = 1024 * 1024
	rlimit.Cur = 1024 * 1024
	err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rlimit)
	if err != nil {
		return err
	}
	err = syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit)
	if err != nil {
		return errors.Wrap(err, "error getting RLIMIT_NOFILE")
	}
	logger.Infof("Ending RLIMIT_NOFILE rlimit.Cur %d, rlimit.Max %d", rlimit.Cur, rlimit.Max)
	return nil
}

func getUpdatedGaneshConfig(config []byte) []byte {
	var (
		tmplBuf bytes.Buffer
		logPath string
	)

	if os.Getpid() == 1 {
		logPath = "/proc/1/fd/1"
	} else {
		logPath = "/tmp/ganesha.log"
	}

	tmplVals := struct {
		LogPath string
	}{
		LogPath: logPath,
	}

	template.Must(template.New("Ganesha_Config").Parse(string(config))).Execute(&tmplBuf, tmplVals)

	return tmplBuf.Bytes()
}
