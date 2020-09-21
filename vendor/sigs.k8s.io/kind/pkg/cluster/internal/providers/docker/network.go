/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package docker

import (
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"net"
	"regexp"
	"strings"

	"sigs.k8s.io/kind/pkg/exec"
)

// This may be overridden by KIND_EXPERIMENTAL_DOCKER_NETWORK env,
// experimentally...
//
// By default currently picking a single network is equivalent to the previous
// behavior *except* that we moved from the default bridge to a user defined
// network because the default bridge is actually special versus any other
// docker network and lacks the emebdded DNS
//
// For now this also makes it easier for apps to join the same network, and
// leaves users with complex networking desires to create and manage their own
// networks.
const fixedNetworkName = "kind"

// ensureNetwork checks if docker network by name exists, if not it creates it
func ensureNetwork(name string) error {
	// TODO: the network might already exist and not have ipv6 ... :|
	// discussion: https://github.com/kubernetes-sigs/kind/pull/1508#discussion_r414594198
	exists, err := checkIfNetworkExists(name)
	if err != nil {
		return err
	}
	// network already exists, we're good
	if exists {
		return nil
	}

	// Generate unique subnet per network based on the name
	// obtained from the ULA fc00::/8 range
	// Make N attempts with "probing" in case we happen to collide
	subnet := generateULASubnetFromName(name, 0)
	err = createNetwork(name, subnet)
	if err == nil {
		// Success!
		return nil
	}

	// On the first try check if ipv6 fails entirely on this machine
	// https://github.com/kubernetes-sigs/kind/issues/1544
	// Otherwise if it's not a pool overlap error, fail
	// If it is, make more attempts below
	if isIPv6UnavailableError(err) {
		// only one attempt, IPAM is automatic in ipv4 only
		return createNetwork(name, "")
	} else if isPoolOverlapError(err) {
		// unknown error ...
		return err
	}

	// keep trying for ipv6 subnets
	const maxAttempts = 5
	for attempt := int32(1); attempt < maxAttempts; attempt++ {
		subnet := generateULASubnetFromName(name, attempt)
		err = createNetwork(name, subnet)
		if err == nil {
			// success!
			return nil
		} else if !isPoolOverlapError(err) {
			// unknown error ...
			return err
		}
	}
	return errors.New("exhausted attempts trying to find a non-overlapping subnet")
}

func createNetwork(name, ipv6Subnet string) error {
	if ipv6Subnet == "" {
		return exec.Command("docker", "network", "create", "-d=bridge", name).Run()
	}
	return exec.Command("docker", "network", "create", "-d=bridge", "--ipv6", "--subnet", ipv6Subnet, name).Run()
}

func checkIfNetworkExists(name string) (bool, error) {
	out, err := exec.Output(exec.Command(
		"docker", "network", "ls",
		"--filter=name=^"+regexp.QuoteMeta(name)+"$",
		"--format={{.Name}}",
	))
	return strings.HasPrefix(string(out), name), err
}

func isIPv6UnavailableError(err error) bool {
	rerr := exec.RunErrorForError(err)
	return rerr != nil && strings.HasPrefix(string(rerr.Output), "Error response from daemon: Cannot read IPv6 setup for bridge")
}

func isPoolOverlapError(err error) bool {
	rerr := exec.RunErrorForError(err)
	return rerr != nil && strings.HasPrefix(string(rerr.Output), "Error response from daemon: Pool overlaps with other one on this address space")
}

// generateULASubnetFromName generate an IPv6 subnet based on the
// name and Nth probing attempt
func generateULASubnetFromName(name string, attempt int32) string {
	ip := make([]byte, 16)
	ip[0] = 0xfc
	ip[1] = 0x00
	h := sha1.New()
	h.Write([]byte(name))
	binary.Write(h, binary.LittleEndian, attempt)
	bs := h.Sum(nil)
	for i := 2; i < 8; i++ {
		ip[i] = bs[i]
	}
	subnet := &net.IPNet{
		IP:   net.IP(ip),
		Mask: net.CIDRMask(64, 128),
	}
	return subnet.String()
}
