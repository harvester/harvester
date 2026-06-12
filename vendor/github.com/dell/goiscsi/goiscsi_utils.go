/*
 *
 * Copyright © 2020-2022 Dell Inc. or its subsidiaries. All Rights Reserved.
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
	"fmt"
	"strings"
)

type sessionParser struct{}

func (sp *sessionParser) Parse(data []byte) []ISCSISession {
	str := string(data)
	lines := strings.Split(str, "\n")

	var result []ISCSISession
	var curSession *ISCSISession
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Target:"):
			session := ISCSISession{}
			session.Target = strings.Fields(line)[1]
			if curSession != nil {
				result = append(result, *curSession)
			}
			curSession = &session
		case curSession == nil:
		case strings.HasPrefix(line, "Current Portal:"):
			curSession.Portal = strings.Split(sessionFieldValue(line), ",")[0]
		case strings.HasPrefix(line, "Iface Transport:"):
			curSession.IfaceTransport = ISCSITransportName(sessionFieldValue(line))
		case strings.HasPrefix(line, "Iface Initiatorname:"):
			curSession.IfaceInitiatorname = sessionFieldValue(line)
		case strings.HasPrefix(line, "Iface IPaddress:"):
			curSession.IfaceIPaddress = sessionFieldValue(line)
		case strings.HasPrefix(line, "SID:"):
			curSession.SID = sessionFieldValue(line)
		case strings.HasPrefix(line, "iSCSI Connection State:"):
			curSession.ISCSIConnectionState = ISCSIConnectionState(sessionFieldValue(line))
		case strings.HasPrefix(line, "iSCSI Session State:"):
			curSession.ISCSISessionState = ISCSISessionState(sessionFieldValue(line))
		case strings.HasPrefix(line, "username:"):
			curSession.Username = sessionFieldValue(line)
		case strings.HasPrefix(line, "password:"):
			curSession.Password = sessionFieldValue(line)
		case strings.HasPrefix(line, "username_in:"):
			curSession.UsernameIn = sessionFieldValue(line)
		case strings.HasPrefix(line, "password_in:"):
			curSession.PasswordIn = sessionFieldValue(line)
		}
	}
	if curSession != nil {
		result = append(result, *curSession)
	}
	return result
}

func sessionFieldValue(s string) string {
	_, value := fieldKeyValue(s, ":")
	return value
}

func nodeFieldKeyValue(s string) (string, string) {
	return fieldKeyValue(s, "=")
}

func fieldKeyValue(s string, sep string) (string, string) {
	var key, value string
	splitted := strings.SplitN(s, sep, 2)
	if len(splitted) > 0 {
		key = strings.Trim(strings.TrimSpace(splitted[0]), sep)
	}
	if len(splitted) > 1 {
		value = replaceEmpty(strings.TrimSpace(splitted[1]))
	}
	return key, value
}

func replaceEmpty(s string) string {
	if s == "<empty>" {
		return ""
	}
	return s
}

type nodeParser struct{}

func (np *nodeParser) Parse(data []byte) []ISCSINode {
	str := string(data)
	lines := strings.Split(str, "\n")
	var result []ISCSINode
	var curNode *ISCSINode
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "# BEGIN RECORD"):
			if curNode != nil {
				result = append(result, *curNode)
			}
			curNode = &ISCSINode{Fields: make(map[string]string)}
		case strings.HasPrefix(line, "# END RECORD"):
			if curNode != nil {
				result = append(result, *curNode)
			}
			curNode = nil
		case curNode == nil:
		case strings.HasPrefix(line, "node.name ="):
			key, value := nodeFieldKeyValue(line)
			curNode.Target = value
			curNode.Fields[key] = value
		case strings.HasPrefix(line, "node.conn[0].address ="):
			key, value := nodeFieldKeyValue(line)
			curNode.Portal = value
			curNode.Fields[key] = value
		case strings.HasPrefix(line, "node.conn[0].port ="):
			key, value := nodeFieldKeyValue(line)
			if curNode.Portal != "" {
				curNode.Portal = fmt.Sprintf("%s:%s", curNode.Portal, value)
			}
			curNode.Fields[key] = value
		default:
			key, value := nodeFieldKeyValue(line)
			curNode.Fields[key] = value
		}
	}
	if curNode != nil {
		result = append(result, *curNode)
	}
	return result
}
