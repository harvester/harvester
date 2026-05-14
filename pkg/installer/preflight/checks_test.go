package preflight

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

type fakeOutput struct {
	output string
	rc     int
}

var (
	execOutputs = map[string]fakeOutput{
		"nproc 4":        {"4\n", 0},
		"nproc 8":        {"8\n", 0},
		"nproc 16":       {"16\n", 0},
		"kvm":            {"kvm\n", 0},
		"metal":          {"none\n", 1},
		"dmidecode-fail": {"", 1},
		"dmidecode-8GiB": {`# dmidecode 3.4
			Getting SMBIOS data from sysfs.
			SMBIOS 3.0.0 present.

			Handle 0x1300, DMI type 19, 31 bytes
			Memory Array Mapped Address
				Starting Address: 0x00000000000
				Ending Address: 0x0007FFFFFFF
				Range Size: 2 GB
				Physical Array Handle: 0x1000
				Partition Width: 1

			Handle 0x1301, DMI type 19, 31 bytes
			Memory Array Mapped Address
				Starting Address: 0x00100000000
				Ending Address: 0x0027FFFFFFF
				Range Size: 6 GB
				Physical Array Handle: 0x1000
				Partition Width: 1`, 0},
		"dmidecode-32GiB": {`# dmidecode 3.4
			Getting SMBIOS data from sysfs.
			SMBIOS 3.0.0 present.

			Handle 0x1300, DMI type 19, 31 bytes
			Memory Array Mapped Address
				Starting Address: 0x00000000000
				Ending Address: 0x0007FFFFFFF
				Range Size: 2 GB
				Physical Array Handle: 0x1000
				Partition Width: 1

			Handle 0x1301, DMI type 19, 31 bytes
			Memory Array Mapped Address
				Starting Address: 0x00100000000
				Ending Address: 0x0087FFFFFFF
				Range Size: 30 GB
				Physical Array Handle: 0x1000
				Partition Width: 1`, 0},
		"dmidecode-64GiB": {`# dmidecode 3.5
			Getting SMBIOS data from sysfs.
			SMBIOS 2.8 present.

			Handle 0x0034, DMI type 19, 31 bytes
			Memory Array Mapped Address
				Starting Address: 0x00000000000
				Ending Address: 0x00FFFFFFFFF
				Range Size: 64 GB
				Physical Array Handle: 0x002F
				Partition Width: 8`, 0},
	}
)

// It turns out to be really irritating to mock exec.Command().
// The trick here is: in normal execution (i.e. not under test),
// execCommand is a pointer to exec.Command(), so it just runs as
// usual.  When under test, we replace execCommand with a function
// that in turn calls _this_ function, with one of the keys in the
// execOutputs above.  This function then runs the TestHelperProcess
// test, passing through that key...
func fakeExecCommand(command string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

// ...and TestHelperProcess looks up the key in execOutputs, and
// behaves as mocked, i.e. it will print the fake output to STDOUT,
// and will exit with the fake return code.
// One irritating subtlety here is that the `go test` framework
// may emit junk to STDERR, so this mocking technique only really
// works for mocking return codes and content on STDOUT.  Still,
// that's OK for our purposes here.
func TestHelperProcess(_ *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	if len(args) < 1 {
		os.Exit(1)
	}

	output, ok := execOutputs[args[0]]
	if !ok {
		os.Exit(1)
	}

	fmt.Print(output.output)
	os.Exit(output.rc)
}

func TestCPUCheck(t *testing.T) {
	defer func() { execCommand = exec.Command }()

	expectedOutputs := map[string]string{
		"nproc 4":  "Only 4 CPU cores detected. Harvester requires at least 8 cores for testing and 16 for production use.",
		"nproc 8":  "8 CPU cores detected. Harvester requires at least 16 cores for production use.",
		"nproc 16": "",
	}

	check := CPUCheck{}
	for key, expectedOutput := range expectedOutputs {
		execCommand = func(_ string, _ ...string) *exec.Cmd {
			return fakeExecCommand(key)
		}
		msg, err := check.Run()
		assert.Nil(t, err)
		assert.Equal(t, expectedOutput, msg)
	}
}

func TestVirtCheck(t *testing.T) {
	defer func() { execCommand = exec.Command }()

	expectedOutputs := map[string]string{
		"kvm":   "System is virtualized (kvm) which is not supported in production.",
		"metal": "",
	}

	check := VirtCheck{}
	for key, expectedOutput := range expectedOutputs {
		execCommand = func(_ string, _ ...string) *exec.Cmd {
			return fakeExecCommand(key)
		}
		msg, err := check.Run()
		assert.Nil(t, err)
		assert.Equal(t, expectedOutput, msg)
	}

}

func TestMemoryCheckDmiDecode(t *testing.T) {
	defer func() { execCommand = exec.Command }()

	expectedOutputs := map[string]string{
		"dmidecode-8GiB":  "Only 8GiB RAM detected. Harvester requires at least 32GiB for testing and 64GiB for production use.",
		"dmidecode-32GiB": "32GiB RAM detected. Harvester requires at least 64GiB for production use.",
		"dmidecode-64GiB": "",
	}

	check := MemoryCheck{}
	for key, expectedOutput := range expectedOutputs {
		execCommand = func(_ string, _ ...string) *exec.Cmd {
			return fakeExecCommand(key)
		}
		msg, err := check.Run()
		assert.Nil(t, err)
		assert.Equal(t, expectedOutput, msg)
	}
}

func TestMemoryCheckProcMemInfo(t *testing.T) {
	defaultMemInfo := procMemInfo
	defer func() { procMemInfo = defaultMemInfo }()
	defer func() { execCommand = exec.Command }()

	execCommand = func(_ string, _ ...string) *exec.Cmd {
		return fakeExecCommand("dmidecode-fail")
	}

	expectedOutputs := map[string]string{
		"./testdata/meminfo-512MiB": "Only 447MiB RAM detected. Harvester requires at least 32GiB for testing and 64GiB for production use.",
		"./testdata/meminfo-32GiB":  "31GiB RAM detected. Harvester requires at least 64GiB for production use.",
		"./testdata/meminfo-64GiB":  "",
	}

	check := MemoryCheck{}
	for file, expectedOutput := range expectedOutputs {
		procMemInfo = file
		msg, err := check.Run()
		assert.Nil(t, err)
		assert.Equal(t, expectedOutput, msg)
	}
}

func TestKVMHostCheck(t *testing.T) {
	defaultDevKvm := devKvm
	defer func() { devKvm = defaultDevKvm }()

	expectedOutputs := map[string]string{
		"./testdata/dev-kvm-does-not-exist": "Harvester requires hardware-assisted virtualization, but /dev/kvm does not exist.",
		"./testdata/dev-kvm":                "",
	}

	check := KVMHostCheck{}
	for file, expectedOutput := range expectedOutputs {
		devKvm = file
		msg, err := check.Run()
		assert.Nil(t, err)
		assert.Equal(t, expectedOutput, msg)
	}
}

func TestNetworkSpeedCheck(t *testing.T) {
	defaultSysClassNetDevSpeed := sysClassNetDevSpeed
	defer func() { sysClassNetDevSpeed = defaultSysClassNetDevSpeed }()

	expectedOutputs := map[string]string{
		"./testdata/%s-speed-100":   "Link speed of eth0 is only 100Mpbs. Harvester requires at least 1Gbps for testing and 10Gbps for production use.",
		"./testdata/%s-speed-1000":  "Link speed of eth0 is 1Gbps. Harvester requires at least 10Gbps for production use.",
		"./testdata/%s-speed-2500":  "Link speed of eth0 is 2.5Gbps. Harvester requires at least 10Gbps for production use.",
		"./testdata/%s-speed-10000": "",
	}

	check := NetworkSpeedCheck{"eth0"}
	for file, expectedOutput := range expectedOutputs {
		sysClassNetDevSpeed = file
		msg, err := check.Run()
		assert.Nil(t, err)
		assert.Equal(t, expectedOutput, msg)
	}
}
