/*
 *
 * Copyright © 2019-2022 Dell Inc. or its subsidiaries. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package goiscsi

import (
	"errors"
	"fmt"
	"strconv"
)

const (
	// MockNumberOfInitiators controls the number of initiators found in mock mode
	MockNumberOfInitiators = "numberOfInitiators"
	// MockNumberOfTargets controls the number of targets found in mock mode
	MockNumberOfTargets = "numberOfTargets"
	// MockNumberOfSessions controls the number of  iSCIS sessions found in mock mode
	MockNumberOfSessions = "numberOfSession"
	// MockNumberOfNodes controls the number of  iSCIS sessions found in mock mode
	MockNumberOfNodes = "numberOfNode"
)

// GOISCSIMock is a struct controlling induced errors
var GOISCSIMock struct {
	InduceDiscoveryError          bool
	InduceInitiatorError          bool
	InduceLoginError              bool
	InduceLogoutError             bool
	InduceRescanError             bool
	InduceGetSessionsError        bool
	InduceGetNodesError           bool
	InduceCreateOrUpdateNodeError bool
	InduceDeleteNodeError         bool
	InduceSetCHAPError            bool
}

// MockISCSI provides a mock implementation of an iscsi client
type MockISCSI struct {
	ISCSIType
}

// NewMockISCSI returns an mock ISCSI client
func NewMockISCSI(opts map[string]string) *MockISCSI {
	iscsi := MockISCSI{
		ISCSIType: ISCSIType{
			mock:    true,
			options: opts,
		},
	}

	return &iscsi
}

func getOptionAsInt(opts map[string]string, key string) int64 {
	v, _ := strconv.ParseInt(opts[key], 10, 64)
	return v
}

func (iscsi *MockISCSI) discoverTargets(address string, _ bool) ([]ISCSITarget, error) {
	if GOISCSIMock.InduceDiscoveryError {
		return []ISCSITarget{}, errors.New("discoverTargets induced error")
	}
	mockedTargets := make([]ISCSITarget, 0)
	count := getOptionAsInt(iscsi.options, MockNumberOfTargets)
	if count == 0 {
		count = 1
	}

	for idx := 0; idx < int(count); idx++ {
		tgt := fmt.Sprintf("%05d", idx)
		mockedTargets = append(mockedTargets,
			ISCSITarget{
				Portal:   address + ":3260",
				GroupTag: "0",
				Target:   "iqn.1992-04.com.mock:600009700bcbb70e32870174000" + tgt,
			})
	}

	// send back a slice of targets
	return mockedTargets, nil
}

func (iscsi *MockISCSI) getInitiators(_ string) ([]string, error) {
	if GOISCSIMock.InduceInitiatorError {
		return []string{}, errors.New("getInitiators induced error")
	}

	mockedInitiators := make([]string, 0)
	count := getOptionAsInt(iscsi.options, MockNumberOfInitiators)
	if count == 0 {
		count = 1
	}

	for idx := 0; idx < int(count); idx++ {
		init := fmt.Sprintf("%05d", idx)
		mockedInitiators = append(mockedInitiators,
			"iqn.1993-08.com.mock:01:00000000"+init)
	}
	return mockedInitiators, nil
}

func (iscsi *MockISCSI) performLogin(_ ISCSITarget) error {
	if GOISCSIMock.InduceLoginError {
		return errors.New("iSCSI Login induced error")
	}

	return nil
}

func (iscsi *MockISCSI) performLogout(_ ISCSITarget) error {
	if GOISCSIMock.InduceLogoutError {
		return errors.New("iSCSI Logout induced error")
	}

	return nil
}

func (iscsi *MockISCSI) performRescan() error {
	if GOISCSIMock.InduceRescanError {
		return errors.New("iSCSI Rescan induced error")
	}

	return nil
}

func (iscsi *MockISCSI) getSessions() ([]ISCSISession, error) {
	if GOISCSIMock.InduceGetSessionsError {
		return []ISCSISession{}, errors.New("getSessions induced error")
	}

	var sessions []ISCSISession
	count := getOptionAsInt(iscsi.options, MockNumberOfSessions)
	if count == 0 {
		count = 1
	}
	for idx := 0; idx < int(count); idx++ {
		init := fmt.Sprintf("%05d", idx)
		session := ISCSISession{}
		session.Target = fmt.Sprintf("iqn.2015-10.com.dell:dellemc-foobar-123-a-7ceb34a%d", idx)
		session.Portal = fmt.Sprintf("192.168.1.%d", idx)
		session.IfaceInitiatorname = "iqn.1993-08.com.mock:01:00000000" + init
		session.IfaceTransport = ISCSITransportNameTCP
		session.ISCSIConnectionState = ISCSIConnectionStateINLOGIN
		session.ISCSISessionState = ISCSISessionStateLOGGEDIN
		session.IfaceIPaddress = "192.168.1.10"
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func (iscsi *MockISCSI) getNodes() ([]ISCSINode, error) {
	if GOISCSIMock.InduceGetNodesError {
		return []ISCSINode{}, errors.New("getSessions induced error")
	}

	var nodes []ISCSINode
	count := getOptionAsInt(iscsi.options, MockNumberOfNodes)
	if count == 0 {
		count = 1
	}
	for idx := 0; idx < int(count); idx++ {
		node := ISCSINode{}
		node.Target = fmt.Sprintf("iqn.2015-10.com.dell:dellemc-foobar-123-a-7ceb34a%d", idx)
		node.Portal = fmt.Sprintf("192.168.1.%d", idx)
		node.Fields = make(map[string]string)
		node.Fields["node.session.scan"] = "auto"
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (iscsi *MockISCSI) newNode(_ ISCSITarget, _ map[string]string) error {
	if GOISCSIMock.InduceCreateOrUpdateNodeError {
		return errors.New("newNode induced error")
	}
	if GOISCSIMock.InduceSetCHAPError {
		return errors.New("set CHAP induced error")
	}
	return nil
}

func (iscsi *MockISCSI) deleteNode(_ ISCSITarget) error {
	if GOISCSIMock.InduceDeleteNodeError {
		return errors.New("newNode induced error")
	}
	return nil
}

// ====================================================================
// Architecture agnostic code for the mock implementation

// DiscoverTargets runs an iSCSI discovery and returns a list of targets.
func (iscsi *MockISCSI) DiscoverTargets(address string, login bool) ([]ISCSITarget, error) {
	return iscsi.discoverTargets(address, login)
}

// GetInitiators returns a list of initiators on the local system.
func (iscsi *MockISCSI) GetInitiators(filename string) ([]string, error) {
	return iscsi.getInitiators(filename)
}

// PerformLogin will attempt to log into an iSCSI target
func (iscsi *MockISCSI) PerformLogin(target ISCSITarget) error {
	return iscsi.performLogin(target)
}

// PerformLogout will attempt to log out of an iSCSI target
func (iscsi *MockISCSI) PerformLogout(target ISCSITarget) error {
	return iscsi.performLogout(target)
}

// PerformRescan will will rescan targets known to current sessions
func (iscsi *MockISCSI) PerformRescan() error {
	return iscsi.performRescan()
}

// GetSessions will query iSCSI session info
func (iscsi *MockISCSI) GetSessions() ([]ISCSISession, error) {
	return iscsi.getSessions()
}

// GetNodes will query iSCSI session info
func (iscsi *MockISCSI) GetNodes() ([]ISCSINode, error) {
	return iscsi.getNodes()
}

// CreateOrUpdateNode creates new or update existing iSCSI node in iscsid database
func (iscsi *MockISCSI) CreateOrUpdateNode(target ISCSITarget, options map[string]string) error {
	return iscsi.newNode(target, options)
}

// DeleteNode delete iSCSI node from iscsid database
func (iscsi *MockISCSI) DeleteNode(target ISCSITarget) error {
	return iscsi.deleteNode(target)
}

// SetCHAPCredentials will set CHAP credentials
func (iscsi *MockISCSI) SetCHAPCredentials(target ISCSITarget, username, password string) error {
	options := make(map[string]string)
	options["node.session.auth.authmethod"] = "CHAP"
	options["node.session.auth.username"] = username
	options["node.session.auth.password"] = password
	return iscsi.newNode(target, options)
}
