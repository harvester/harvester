package nfs

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/pkg/errors"

	"github.com/longhorn/longhorn-share-manager/pkg/util"
)

type ExportMap struct {
	idToVolume map[uint16]string
	volumeToid map[string]uint16
}

type Exporter struct {
	*ExportMap

	configPath string
	exportPath string

	mapMutex  sync.RWMutex
	fileMutex sync.Mutex
}

var exportRegex = regexp.MustCompile("Export_Id = ([0-9]+);#Volume=(.+)")

func NewExporter(configPath, exportPath string) (*Exporter, error) {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, errors.Wrapf(err, "nfs server config file %v does not exist", configPath)
	}

	exportIDs, err := getIDsFromConfig(configPath)
	if err != nil {
		return nil, err
	}

	volToID := map[string]uint16{}
	for id, vol := range exportIDs {
		volToID[vol] = id
	}

	return &Exporter{
		ExportMap: &ExportMap{
			idToVolume: exportIDs,
			volumeToid: volToID,
		},

		configPath: configPath,
		exportPath: exportPath,

		mapMutex:  sync.RWMutex{},
		fileMutex: sync.Mutex{},
	}, nil
}

// GetExportMap returns a copy of the export map
func (e *Exporter) GetExportMap() ExportMap {
	e.mapMutex.RLock()
	defer e.mapMutex.RUnlock()

	volToID := map[string]uint16{}
	for vol, id := range e.ExportMap.volumeToid {
		volToID[vol] = id
	}

	idToVol := map[uint16]string{}
	for id, vol := range e.ExportMap.idToVolume {
		idToVol[id] = vol
	}

	return ExportMap{
		idToVolume: idToVol,
		volumeToid: volToID,
	}
}

// GetExport returns the export id for a volume, where 0 equals unexported
func (e *Exporter) GetExport(volume string) uint16 {
	e.mapMutex.RLock()
	defer e.mapMutex.RUnlock()
	return e.volumeToid[volume]
}

// claimID generates a unique export id for a volume, a volume can only be exported once
func (e *Exporter) claimID(volume string) uint16 {
	e.mapMutex.Lock()
	defer e.mapMutex.Unlock()

	if id, ok := e.volumeToid[volume]; ok {
		return id
	}

	id := uint16(1)
	for ; ; id++ {
		if _, ok := e.idToVolume[id]; !ok {
			e.idToVolume[id] = volume
			e.volumeToid[volume] = id
			break
		}
	}

	return id
}

func (e *Exporter) deleteID(id uint16) {
	e.mapMutex.Lock()
	defer e.mapMutex.Unlock()

	if vol, ok := e.idToVolume[id]; ok {
		delete(e.volumeToid, vol)
	}

	delete(e.idToVolume, id)
}

func (e *Exporter) CreateExport(volume string) (uint16, error) {
	if id := e.GetExport(volume); id != 0 {
		return id, nil
	}

	exportID := e.claimID(volume)
	block := generateExportBlock(e.exportPath, volume, exportID)

	if err := e.addToConfig(block); err != nil {
		e.deleteID(exportID)
		return 0, errors.Wrapf(err, "error adding export block %s to config %s", block, e.configPath)
	}
	return exportID, nil
}

func (e *Exporter) DeleteExport(volume string) error {
	id, ok := e.volumeToid[volume]
	if !ok {
		return nil
	}

	// TODO: write a lexer and parser for the export config
	// 	instead of doing these string manipulations
	block := generateExportBlock(e.exportPath, volume, id)
	if err := e.removeFromConfig(block); err != nil {
		return err
	}

	e.deleteID(id)
	return nil
}

func (e *Exporter) ReloadExport() error {
	processName := "ganesha.nfsd"

	process, err := util.FindProcessByName(processName)
	if err != nil {
		return errors.Wrapf(err, "failed to find process %s", processName)
	}

	err = process.Signal(syscall.SIGHUP)
	if err != nil {
		return fmt.Errorf("failed to send SIGHUP to process %s", processName)
	}

	return nil
}

func generateExportBlock(exportBase, volume string, id uint16) string {
	squash := "None"
	secType := "sys"
	pseudoPath := filepath.Join("/", volume)
	exportPath := filepath.Join(exportBase, volume)
	exportID := strconv.FormatUint(uint64(id), 10)
	volumeMarker := "#Volume=" + volume

	return "\nEXPORT\n{\n" +
		"\tExport_Id = " + exportID + ";" + volumeMarker + "\n" +
		"\tPath = " + exportPath + ";\n" +
		"\tPseudo = " + pseudoPath + ";\n" +
		"\tProtocols = 4;\n" +
		"\tTransports = TCP;\n" +
		"\tAccess_Type = RW;\n" +
		"\tSquash = " + squash + ";\n" +
		"\tSecType = " + secType + ";\n" +
		"\tFilesystem_id = " + exportID + "." + "0" + ";\n" +
		"\tFSAL {\n\t\tName = VFS;\n\t}\n}\n"
}

// getIDsFromConfig populates a map with existing ids found in the given config
// file using the given regexp. Regexp must have a "digits" submatch.
func getIDsFromConfig(configPath string) (map[uint16]string, error) {
	ids := map[uint16]string{}
	config, err := os.ReadFile(configPath)
	if err != nil {
		return ids, err
	}

	allMatches := exportRegex.FindAllSubmatch(config, -1)
	for _, match := range allMatches {
		// there should be the full match a submatch for id and volume
		// only the pseudo export has no volume marker
		if len(match) == 3 {
			digits := string(match[1])
			volume := string(match[2])
			if id, err := strconv.ParseUint(digits, 10, 16); err == nil {
				ids[uint16(id)] = volume
			}
		}
	}

	return ids, nil
}

func (e *Exporter) addToConfig(block string) error {
	e.fileMutex.Lock()
	defer e.fileMutex.Unlock()

	config, err := os.OpenFile(e.configPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer config.Close()

	if _, err = config.WriteString(block); err != nil {
		return err
	}
	if err = config.Sync(); err != nil {
		return err
	}
	return nil
}

func (e *Exporter) removeFromConfig(block string) error {
	e.fileMutex.Lock()
	defer e.fileMutex.Unlock()

	config, err := os.ReadFile(e.configPath)
	if err != nil {
		return err
	}

	newConfig := strings.Replace(string(config), block, "", -1)
	err = os.WriteFile(e.configPath, []byte(newConfig), 0)
	if err != nil {
		return err
	}

	return nil
}
