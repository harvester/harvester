package utils

import (
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
)

const (
	MaxVlanID            = 4094
	MinVlanID            = 0
	MinTrunkVlanID       = 1
	DefaultVlanID        = 1
	VlanIDCount          = 4096
	VlanIDStringJoinChar = ","
)

type VlanIDSet struct {
	vlanCount   uint32 // store the vid count on the vidSet
	vidSet      []bool // store vlan list for trunk mode
	vid         int    // store the vid when this is for the the untag/tag mode
	isTrunkMode bool   // when it is trunk mode, the vids are stored on vidSet, in other case, stored in vid
}

func (vis *VlanIDSet) SetVID(vid int) error {
	if vid < MinVlanID || vid > MaxVlanID {
		return fmt.Errorf("vlan %v is out of range [%v .. %v]", vid, MinVlanID, MaxVlanID)
	}
	vis._setVID(vid)
	return nil
}

// refer pkg/network/iface/vlan.go, which uses uint16 as vid
func (vis *VlanIDSet) SetUint16VID(vid uint16) error {
	// skip G115
	return vis.SetVID(int(vid)) //nolint:gosec
}

// caller has ensured the vid is in range
func (vis *VlanIDSet) _setVID(vid int) {
	if vid == MinVlanID {
		// skip set vid 0 in any case
		return
	}

	if !vis.isTrunkMode {
		vis.vid = vid
		return
	}

	// trunk mode
	if !vis.vidSet[vid] {
		vis.vidSet[vid] = true
		vis.vlanCount++
	}
}

// only called when vidset is in trunk mode
func (vis *VlanIDSet) _unsetVID(vid int) {
	if !vis.isTrunkMode || !vis.vidSet[vid] {
		return
	}

	vis.vidSet[vid] = false
	vis.vlanCount--
}

// merge another VlandIDSet to current
func (vis *VlanIDSet) Append(other *VlanIDSet) error {
	if !vis.isTrunkMode {
		return fmt.Errorf("target vidset is not trunk mode, can't append")
	}
	if other == nil {
		return nil
	}
	if !other.isTrunkMode {
		return vis.SetVID(other.vid)
	}
	// if there is no vid on the list, skip the iterating
	if other.vlanCount == 0 {
		return nil
	}
	// iterate the trunk vids
	for i := DefaultVlanID; i <= MaxVlanID; i++ {
		if other.vidSet[i] {
			if err := vis.SetVID(i); err != nil {
				return err
			}
		}
	}
	return nil
}

func (vis *VlanIDSet) VidSetToString() string {
	if !vis.isTrunkMode {
		if vis.vid == MinVlanID {
			return ""
		}
		return strconv.Itoa(vis.vid)
	}

	target := make([]string, vis.vlanCount)
	k := 0
	for i := DefaultVlanID; i <= MaxVlanID; i++ {
		if vis.vidSet[i] {
			target[k] = strconv.Itoa(i)
			k++
		}
	}
	return strings.Join(target, VlanIDStringJoinChar)
}

// according to current and the existing vlandidset, compute the to be added and removed vidset
func (vis *VlanIDSet) Diff(existing *VlanIDSet) (added, removed *VlanIDSet, err error) {
	if existing == nil {
		return nil, nil, fmt.Errorf("can't run diff on an empty vidset")
	}
	if !vis.isTrunkMode || !existing.isTrunkMode {
		return nil, nil, fmt.Errorf("can only run diff between trunk mode vidset")
	}
	added = NewVlanIDSet()
	removed = NewVlanIDSet()
	// default vid (1) is always there, don't add or remove it
	for i := DefaultVlanID + 1; i <= MaxVlanID; i++ {
		if vis.vidSet[i] != existing.vidSet[i] {
			if vis.vidSet[i] {
				added._setVID(i)
			} else {
				removed._setVID(i)
			}
		}
	}
	err = nil
	return
}

func (vis *VlanIDSet) VidSetToStringHash() (str, hash string) {
	str = vis.VidSetToString()
	bs := sha256.Sum256([]byte(str))
	hash = fmt.Sprintf("%x", bs)
	return
}

func (vis *VlanIDSet) GetVlanCount() uint32 {
	if vis.isTrunkMode {
		return vis.vlanCount
	}
	if vis.vid > MinVlanID {
		return 1 // tag mode has 1 vid
	}
	return 0 // untag mode has 0 vid
}

// walk vids in range [2..4094]
func (vis *VlanIDSet) WalkVIDs(name string, callback func(vid uint16) error) error {
	if !vis.isTrunkMode {
		// do not callback on vid 0 and 1
		if vis.vid <= DefaultVlanID {
			return nil
		}
		if err := callback(uint16(vis.vid)); err != nil { // nolint: gosec
			return fmt.Errorf("failed to walk %v on vid %v, error: %w ", name, vis.vid, err)
		}
		return nil
	}

	// if there is no vid on the list, skip the iterating
	if vis.vlanCount == 0 {
		return nil
	}
	for i := DefaultVlanID + 1; i <= MaxVlanID; i++ {
		if vis.vidSet[i] {
			if err := callback(uint16(i)); err != nil { // nolint: gosec
				return fmt.Errorf("failed to walk %v on trunk vid %v, error: %w ", name, i, err)
			}
		}
	}
	return nil
}

// when run Append() or Diff(), if the vidset is in single mode, convert it to trunk mode first
func (vis *VlanIDSet) ConvertToTrunkMode() {
	// already in trunk mode
	if vis.isTrunkMode {
		return
	}
	vis.vidSet = make([]bool, VlanIDCount)
	vis.isTrunkMode = true

	vis._setVID(vis.vid)
}

// trunk mode
func NewVlanIDSet() *VlanIDSet {
	vis := &VlanIDSet{
		vidSet:      make([]bool, VlanIDCount),
		isTrunkMode: true,
	}
	return vis
}

// for l2 vlan & untag, it can hold only 1 vid
func NewVlanIDSetFromSingleVID(vid int) (*VlanIDSet, error) {
	vis := &VlanIDSet{
		isTrunkMode: false,
	}
	if err := vis.SetVID(vid); err != nil {
		return nil, err
	}
	return vis, nil
}
