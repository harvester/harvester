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
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	// ChrootDirectory allows the iscsiadm commands to be run within a chrooted path, helpful for containerized services
	ChrootDirectory = "chrootDirectory"
	// DefaultInitiatorNameFile is the default file which contains the initiator names
	DefaultInitiatorNameFile = "/etc/iscsi/initiatorname.iscsi"

	// ISCSINoObjsFoundExitCode exit code indicates that no records/targets/sessions/portals
	// found to execute operation on
	iSCSINoObjsFoundExitCode = 21
	// Timeout for iscsiadm command to execute
	Timeout = 30
)

// LinuxISCSI provides many iSCSI-specific functions.
type LinuxISCSI struct {
	ISCSIType
	sessionParser iSCSISessionParser
	nodeParser    iSCSINodeParser
}

// NewLinuxISCSI returns an LinuxISCSI client
func NewLinuxISCSI(opts map[string]string) *LinuxISCSI {
	iscsi := LinuxISCSI{
		ISCSIType: ISCSIType{
			mock:    false,
			options: opts,
		},
	}
	iscsi.sessionParser = &sessionParser{}
	iscsi.nodeParser = &nodeParser{}

	return &iscsi
}

func (iscsi *LinuxISCSI) getChrootDirectory() string {
	s := iscsi.options[ChrootDirectory]
	if s == "" {
		s = "/"
	}
	return s
}

func (iscsi *LinuxISCSI) buildISCSICommand(cmd []string) []string {
	if iscsi.getChrootDirectory() == "/" {
		return cmd
	}
	command := []string{"chroot", iscsi.getChrootDirectory()}
	command = append(command, cmd...)
	return command
}

// DiscoverTargets runs an iSCSI discovery and returns a list of targets.
func (iscsi *LinuxISCSI) DiscoverTargets(address string, login bool) ([]ISCSITarget, error) {
	return iscsi.discoverTargets(address, login)
}

func (iscsi *LinuxISCSI) discoverTargets(address string, login bool) ([]ISCSITarget, error) {
	// iSCSI discovery is done via the iscsiadm cli
	// iscsiadm -m discovery -t st --portal <target>

	// validate for valid address
	err := validateIPAddress(address)
	if err != nil {
		fmt.Printf("\nError invalid address %s: %v", address, err)
		return []ISCSITarget{}, err
	}
	exe := iscsi.buildISCSICommand([]string{"iscsiadm", "-m", "discovery", "-t", "st", "--portal", address})
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(Timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe[0], exe[1:]...) // #nosec G204

	out, err := cmd.Output()
	if err != nil {
		fmt.Printf("\nError discovering %s: %v", address, err)
		return []ISCSITarget{}, err
	}

	targets := make([]ISCSITarget, 0)

	for _, line := range strings.Split(string(out), "\n") {
		// one line of the output should look like:
		// 1.1.1.1:3260,0 iqn.1992-04.com.emc:600009700bcbb70e3287017400000001
		// Portal,GroupTag Target
		tokens := strings.Split(line, " ")
		// make sure we got two tokens
		if len(tokens) == 2 {
			addrtag := strings.Split(line, " ")[0]
			tgt := strings.Split(line, " ")[1]
			targets = append(targets,
				ISCSITarget{
					Portal:   strings.Split(addrtag, ",")[0],
					GroupTag: strings.Split(addrtag, ",")[1],
					Target:   tgt,
				})
		}
	}
	// log into the target if asked
	if login {
		for _, t := range targets {
			_ = iscsi.PerformLogin(t)
		}
	}

	return targets, nil
}

// GetInitiators returns a list of initiators on the local system.
func (iscsi *LinuxISCSI) GetInitiators(filename string) ([]string, error) {
	return iscsi.getInitiators(filename)
}

func (iscsi *LinuxISCSI) getInitiators(filename string) ([]string, error) {
	// a slice of filename, which might exist and define the iSCSI initiators
	initiatorConfig := []string{}
	iqns := []string{}

	if filename == "" {
		// add default filename(s) here
		// /etc/iscsi/initiatorname.iscsi is the proper file for CentOS, RedHat, Debian, Ubuntu
		if iscsi.getChrootDirectory() != "/" {
			initiatorConfig = append(initiatorConfig, iscsi.getChrootDirectory()+"/"+DefaultInitiatorNameFile)
		} else {
			initiatorConfig = append(initiatorConfig, DefaultInitiatorNameFile)
		}
	} else {
		initiatorConfig = append(initiatorConfig, filename)
	}

	// for each initiatior config file
	for _, init := range initiatorConfig {
		// make sure the file exists
		_, err := os.Stat(init)
		if err != nil {
			return []string{}, err
		}

		// get the contents of the initiator config file
		cmd, err := os.ReadFile(filepath.Clean(init))
		if err != nil {
			fmt.Printf("Error gathering initiator names: %v", err)
			return nil, err
		}
		lines := strings.Split(string(cmd), "\n")
		for _, l := range lines {
			// remove all whitespace to catch different formatting
			l = strings.Join(strings.Fields(l), "")
			if strings.HasPrefix(l, "InitiatorName=") {
				iqns = append(iqns, strings.Split(l, "=")[1])
			}
		}
	}

	return iqns, nil
}

// PerformLogin will attempt to log into an iSCSI target
func (iscsi *LinuxISCSI) PerformLogin(target ISCSITarget) error {
	return iscsi.performLogin(target)
}

func (iscsi *LinuxISCSI) performLogin(target ISCSITarget) error {
	// iSCSI login is done via the iscsiadm cli
	// iscsiadm -m node -T <target> --portal <address> -l

	err := validateIPAddress(target.Portal)
	if err != nil {
		fmt.Printf("\nError invalid portal address %s: %v", target.Portal, err)
		return err
	}

	err = validateIQN(target.Target)
	if err != nil {
		fmt.Printf("\nError invalid IQN Target %s: %v", target.Target, err)
		return err
	}

	exe := iscsi.buildISCSICommand([]string{"iscsiadm", "-m", "node", "-T", target.Target, "--portal", target.Portal, "-l"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(Timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe[0], exe[1:]...) // #nosec G204

	_, err = cmd.Output()
	if err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			// iscsiadm exited with an exit code != 0
			iscsiResult := -1
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				iscsiResult = status.ExitStatus()
			}
			if iscsiResult == 15 {
				// session already exists
				// do not treat this as a failure
				err = nil
			} else {
				fmt.Printf("\niscsiadm login failure: %v", err)
			}
		} else {
			fmt.Printf("\nError logging %s at %s: %v", target.Target, target.Portal, err)
		}

		if err != nil {
			fmt.Printf("\nError logging %s at %s: %v", target.Target, target.Portal, err)
			return err
		}
	}

	return nil
}

// PerformLogout will attempt to log out of an iSCSI target
func (iscsi *LinuxISCSI) PerformLogout(target ISCSITarget) error {
	return iscsi.performLogout(target)
}

func (iscsi *LinuxISCSI) performLogout(target ISCSITarget) error {
	// iSCSI login is done via the iscsiadm cli
	// iscsiadm -m node -T <target> --portal <address> -l
	err := validateIPAddress(target.Portal)
	if err != nil {
		fmt.Printf("\nError invalid portal address %s: %v", target.Portal, err)
		return err
	}

	err = validateIQN(target.Target)
	if err != nil {
		fmt.Printf("\nError invalid IQN Target %s: %v", target.Target, err)
		return err
	}

	exe := iscsi.buildISCSICommand([]string{"iscsiadm", "-m", "node", "-T", target.Target, "--portal", target.Portal, "--logout"})
	cmd := exec.Command(exe[0], exe[1:]...) // #nosec G204

	_, err = cmd.Output()
	if err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			// iscsiadm exited with an exit code != 0
			iscsiResult := -1
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				iscsiResult = status.ExitStatus()
			}
			if iscsiResult == 15 {
				// session already exists
				// do not treat this as a failure
				err = nil
			} else {
				fmt.Printf("iscsiadm login failure: %v", err)
			}
		} else {
			fmt.Printf("Error logging %s at %s: %v", target.Target, target.Portal, err)
		}

		if err != nil {
			fmt.Printf("Error logging %s at %s: %v", target.Target, target.Portal, err)
			return err
		}
	}

	return nil
}

// PerformRescan will rescan targets known to current sessions
func (iscsi *LinuxISCSI) PerformRescan() error {
	return iscsi.performRescan()
}

func (iscsi *LinuxISCSI) performRescan() error {
	exe := iscsi.buildISCSICommand([]string{"iscsiadm", "-m", "node", "--rescan"})
	cmd := exec.Command(exe[0], exe[1:]...) // #nosec G204

	_, err := cmd.Output()
	if err != nil {
		return err
	}
	return nil
}

// GetSessions will query information about sessions
func (iscsi *LinuxISCSI) GetSessions() ([]ISCSISession, error) {
	exe := iscsi.buildISCSICommand([]string{"iscsiadm", "-m", "session", "-P", "2", "-S"})
	cmd := exec.Command(exe[0], exe[1:]...) // #nosec G204
	output, err := cmd.Output()
	if err != nil {
		if isNoObjsExitCode(err) {
			return []ISCSISession{}, nil
		}
		return []ISCSISession{}, err
	}
	return iscsi.sessionParser.Parse(output), nil
}

// GetNodes will query information about nodes
func (iscsi *LinuxISCSI) GetNodes() ([]ISCSINode, error) {
	exe := iscsi.buildISCSICommand([]string{"iscsiadm", "-m", "node", "-o", "show"})
	cmd := exec.Command(exe[0], exe[1:]...) // #nosec G204
	output, err := cmd.Output()
	if err != nil {
		if isNoObjsExitCode(err) {
			return []ISCSINode{}, nil
		}
		return []ISCSINode{}, err
	}
	return iscsi.nodeParser.Parse(output), nil
}

// SetCHAPCredentials will set CHAP credentials
func (iscsi *LinuxISCSI) SetCHAPCredentials(target ISCSITarget, username, password string) error {
	options := make(map[string]string)
	options["node.session.auth.authmethod"] = "CHAP"
	options["node.session.auth.username"] = username
	options["node.session.auth.password"] = password
	return iscsi.CreateOrUpdateNode(target, options)
}

// CreateOrUpdateNode creates new or update existing iSCSI node in iscsid dm
func (iscsi *LinuxISCSI) CreateOrUpdateNode(target ISCSITarget, options map[string]string) error {
	err := validateIPAddress(target.Portal)
	if err != nil {
		fmt.Printf("\nError invalid portal address %s: %v", target.Portal, err)
		return err
	}

	err = validateIQN(target.Target)
	if err != nil {
		fmt.Printf("\nError invalid IQN Target %s: %v", target.Target, err)
		return err
	}
	baseCmd := iscsi.buildISCSICommand(
		[]string{"iscsiadm", "-m", "node", "-p", target.Portal, "-T", target.Target})

	var commands [][]string

	cmd := exec.Command(baseCmd[0], baseCmd[1:]...) // #nosec G204
	_, err = cmd.Output()
	if err != nil {
		if !isNoObjsExitCode(err) {
			return err
		}
		c := append(append([]string{}, baseCmd...), "-o", "new")
		commands = append(commands, c)
	}

	for k, v := range options {
		c := append(append([]string{}, baseCmd...), "-o", "update", "-n", k, "-v", v)
		commands = append(commands, c)
	}
	for _, command := range commands {
		cmd := exec.Command(command[0], command[1:]...) // #nosec G204
		_, err := cmd.Output()
		if err != nil {
			return err
		}
	}
	return nil
}

// DeleteNode delete iSCSI node from iscsid database
func (iscsi *LinuxISCSI) DeleteNode(target ISCSITarget) error {
	err := validateIPAddress(target.Portal)
	if err != nil {
		fmt.Printf("\nError invalid portal address %s: %v", target.Portal, err)
		return err
	}

	err = validateIQN(target.Target)
	if err != nil {
		fmt.Printf("\nError invalid IQN Target %s: %v", target.Target, err)
		return err
	}
	exe := iscsi.buildISCSICommand(
		[]string{"iscsiadm", "-m", "node", "-p", target.Portal, "-T", target.Target, "-o", "delete"})
	cmd := exec.Command(exe[0], exe[1:]...) // #nosec G204
	_, err = cmd.Output()
	if err != nil {
		if isNoObjsExitCode(err) {
			return nil
		}
		return err
	}
	return nil
}

func isNoObjsExitCode(err error) bool {
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return exitError.ExitCode() == iSCSINoObjsFoundExitCode
		}
	}
	return false
}
