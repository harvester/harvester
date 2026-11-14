package console

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/harvester/harvester/pkg/installer/config"
	"github.com/harvester/harvester/pkg/installer/widgets"
)

func TestDiskPageVisiblePanels(t *testing.T) {
	testCases := []struct {
		name      string
		install   config.Install
		diskCount int
		profile   *installProfile
		want      []string
	}{
		{
			name:      "default node shows the skip decision before its dependent disk fields",
			install:   config.Install{Role: config.RoleDefault, Device: "/dev/sda", DataDisk: "/dev/sda"},
			diskCount: 2,
			want:      []string{skipLonghornDefaultDiskPanel, diskPanel, dataDiskPanel, persistentSizePanel},
		},
		{
			name:      "local skip selection keeps the selector available",
			install:   config.Install{Role: config.RoleDefault, Device: "/dev/sda", SkipLonghornDefaultDisk: true},
			diskCount: 2,
			want:      []string{skipLonghornDefaultDiskPanel, diskPanel},
		},
		{
			name:      "cluster skip policy hides all Longhorn disk controls",
			install:   config.Install{Role: config.RoleDefault, Device: "/dev/sda", SkipLonghornDefaultDisk: true},
			diskCount: 2,
			profile:   &installProfile{SkipLonghornDefaultDisk: true},
			want:      []string{diskPanel},
		},
		{
			name:      "cluster create policy hides only the policy selector",
			install:   config.Install{Role: config.RoleDefault, Device: "/dev/sda", DataDisk: "/dev/sda"},
			diskCount: 2,
			profile:   &installProfile{},
			want:      []string{diskPanel, dataDiskPanel, persistentSizePanel},
		},
		{
			name:      "separate data disk does not show persistent size",
			install:   config.Install{Role: config.RoleDefault, Device: "/dev/sda", DataDisk: "/dev/sdb"},
			diskCount: 2,
			want:      []string{skipLonghornDefaultDiskPanel, diskPanel, dataDiskPanel},
		},
		{
			name:      "single disk keeps the existing persistent size flow",
			install:   config.Install{Role: config.RoleDefault, Device: "/dev/sda", DataDisk: "/dev/sda"},
			diskCount: 1,
			want:      []string{skipLonghornDefaultDiskPanel, diskPanel, persistentSizePanel},
		},
		{
			name:      "witness does not display Longhorn disk options",
			install:   config.Install{Role: config.RoleWitness, Device: "/dev/sda"},
			diskCount: 2,
			want:      []string{diskPanel},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.want, diskPageVisiblePanels(testCase.install, testCase.diskCount, testCase.profile))
		})
	}
}

func TestLonghornDefaultDiskNote(t *testing.T) {
	assert.Equal(t,
		"Note: Longhorn default disk will not be created.",
		longhornDefaultDiskNote(&installProfile{SkipLonghornDefaultDisk: true}),
	)
	assert.Empty(t, longhornDefaultDiskNote(&installProfile{}))
	assert.Empty(t, longhornDefaultDiskNote(nil))
}

func TestPresetConfigDisksDoesNotSetDataDiskWhenSkipping(t *testing.T) {
	console := &Console{config: &config.HarvesterConfig{Install: config.Install{SkipLonghornDefaultDisk: true}}}

	presetConfigDisks(console, []widgets.Option{{Value: "/dev/sda"}})

	assert.Equal(t, "/dev/sda", console.config.Install.Device)
	assert.Empty(t, console.config.Install.DataDisk)
}

func TestJoinNeedsDiskSelection(t *testing.T) {
	assert.True(t, joinNeedsDiskSelection(false))
	assert.False(t, joinNeedsDiskSelection(true))
}

func TestRestoredDataDisk(t *testing.T) {
	testCases := []struct {
		name         string
		previousData string
		installation string
		wantDataDisk string
	}{
		{
			name:         "restores a previously selected separate disk",
			previousData: "/dev/sdb",
			installation: "/dev/sda",
			wantDataDisk: "/dev/sdb",
		},
		{
			name:         "defaults to the installation disk when no previous choice exists",
			installation: "/dev/sda",
			wantDataDisk: "/dev/sda",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.wantDataDisk, restoredDataDisk(testCase.previousData, testCase.installation))
		})
	}
}

func TestNextPanelAfterDataDiskSelection(t *testing.T) {
	testCases := []struct {
		name             string
		installationDisk string
		dataDisk         string
		want             string
	}{
		{
			name:             "same disk continues forward to persistent size",
			installationDisk: "/dev/sda",
			dataDisk:         "/dev/sda",
			want:             persistentSizePanel,
		},
		{
			name:             "separate disk has no additional disk field",
			installationDisk: "/dev/sda",
			dataDisk:         "/dev/sdb",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.want, nextPanelAfterDataDiskSelection(testCase.installationDisk, testCase.dataDisk))
		})
	}
}

func TestDataDisksForLonghornDefaultDiskSelection(t *testing.T) {
	testCases := []struct {
		name               string
		wasSkipping        bool
		isSkipping         bool
		currentDataDisk    string
		rememberedDataDisk string
		installationDisk   string
		wantDataDisk       string
		wantRememberedDisk string
	}{
		{
			name:               "entering skip remembers and clears a separate data disk",
			isSkipping:         true,
			currentDataDisk:    "/dev/sdb",
			installationDisk:   "/dev/sda",
			wantRememberedDisk: "/dev/sdb",
		},
		{
			name:               "confirming skip again retains the remembered data disk",
			wasSkipping:        true,
			isSkipping:         true,
			rememberedDataDisk: "/dev/sdb",
			installationDisk:   "/dev/sda",
			wantRememberedDisk: "/dev/sdb",
		},
		{
			name:               "leaving skip restores the remembered data disk",
			wasSkipping:        true,
			rememberedDataDisk: "/dev/sdb",
			installationDisk:   "/dev/sda",
			wantDataDisk:       "/dev/sdb",
			wantRememberedDisk: "/dev/sdb",
		},
		{
			name:               "confirming create retains the current data disk",
			currentDataDisk:    "/dev/sdc",
			rememberedDataDisk: "/dev/sdb",
			installationDisk:   "/dev/sda",
			wantDataDisk:       "/dev/sdc",
			wantRememberedDisk: "/dev/sdb",
		},
		{
			name:             "leaving skip without a prior choice defaults to the installation disk",
			wasSkipping:      true,
			installationDisk: "/dev/sda",
			wantDataDisk:     "/dev/sda",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			dataDisk, rememberedDataDisk := dataDisksForLonghornDefaultDiskSelection(
				testCase.wasSkipping,
				testCase.isSkipping,
				testCase.currentDataDisk,
				testCase.rememberedDataDisk,
				testCase.installationDisk,
			)

			assert.Equal(t, testCase.wantDataDisk, dataDisk)
			assert.Equal(t, testCase.wantRememberedDisk, rememberedDataDisk)
		})
	}
}

func TestRequiresPersistentSizeValidation(t *testing.T) {
	testCases := []struct {
		name    string
		install config.Install
		want    bool
	}{
		{
			name:    "normal node validates the persistent size",
			install: config.Install{Role: config.RoleDefault},
			want:    true,
		},
		{
			name:    "skipped default disk does not validate a hidden persistent size",
			install: config.Install{Role: config.RoleDefault, SkipLonghornDefaultDisk: true},
		},
		{
			name:    "witness node does not validate a persistent size",
			install: config.Install{Role: config.RoleWitness},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.want, requiresPersistentSizeValidation(testCase.install))
		})
	}
}

func TestDiskSelectionCompleteAfterInstallationDiskConfirmation(t *testing.T) {
	testCases := []struct {
		name    string
		install config.Install
		want    bool
	}{
		{
			name:    "normal node continues to persistent-size selection",
			install: config.Install{Role: config.RoleDefault},
		},
		{
			name:    "skipped default disk completes selection without a persistent size",
			install: config.Install{Role: config.RoleDefault, SkipLonghornDefaultDisk: true},
			want:    true,
		},
		{
			name:    "witness completes selection without a persistent size",
			install: config.Install{Role: config.RoleWitness},
			want:    true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.want, diskSelectionCompleteAfterInstallationDiskConfirmation(testCase.install))
		})
	}
}
