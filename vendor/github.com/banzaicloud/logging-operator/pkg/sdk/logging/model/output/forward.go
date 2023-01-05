// Copyright © 2019 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package output

import (
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/common"
	"github.com/banzaicloud/logging-operator/pkg/sdk/logging/model/types"
	"github.com/banzaicloud/operator-tools/pkg/secret"
)

// +name:"Forward"
// +weight:"200"
type _hugoForward interface{} //nolint:deadcode,unused

// +name:"Forward"
// +url:"https://docs.fluentd.org/output/forward"
// +version:"more info"
// +description:"Forwards events to other fluentd nodes."
// +status:"GA"
type _metaForward interface{} //nolint:deadcode,unused

// +kubebuilder:object:generate=true
type ForwardOutput struct {
	// Server definitions at least one is required
	// +docLink:"Server,#fluentd-server"
	FluentdServers []FluentdServer `json:"servers"`
	// The transport protocol to use [ tcp, tls ]
	Transport string `json:"transport,omitempty"`
	// Change the protocol to at-least-once. The plugin waits the ack from destination's in_forward plugin.
	RequireAckResponse bool `json:"require_ack_response,omitempty"`
	// This option is used when require_ack_response is true. This default value is based on popular tcp_syn_retries. (default: 190)
	AckResponseTimeout int `json:"ack_response_timeout,omitempty"`
	// The timeout time when sending event logs. (default: 60)
	SendTimeout int `json:"send_timeout,omitempty"`
	// The timeout time for socket connect. When the connection timed out during establishment, Errno::ETIMEDOUT is raised.
	ConnectTimeout int `json:"connect_timeout,omitempty"`
	// The wait time before accepting a server fault recovery. (default: 10)
	RecoverWait int `json:"recover_wait,omitempty"`
	// The transport protocol to use for heartbeats. Set "none" to disable heartbeat. [transport, tcp, udp, none]
	HeartbeatType string `json:"heartbeat_type,omitempty"`
	// The interval of the heartbeat packer. (default: 1)
	HeartbeatInterval int `json:"heartbeat_interval,omitempty"`
	// Use the "Phi accrual failure detector" to detect server failure. (default: true)
	PhiFailureDetector bool `json:"phi_failure_detector,omitempty"`
	// The threshold parameter used to detect server faults. (default: 16)
	//`phi_threshold` is deeply related to `heartbeat_interval`. If you are using longer `heartbeat_interval`, please use the larger `phi_threshold`. Otherwise you will see frequent detachments of destination servers. The default value 16 is tuned for `heartbeat_interval` 1s.
	PhiThreshold int `json:"phi_threshold,omitempty"`
	// The hard timeout used to detect server failure. The default value is equal to the send_timeout parameter. (default: 60)
	HardTimeout int `json:"hard_timeout,omitempty"`
	// Set TTL to expire DNS cache in seconds. Set 0 not to use DNS Cache. (default: 0)
	ExpireDnsCache int `json:"expire_dns_cache,omitempty"`
	// Enable client-side DNS round robin. Uniform randomly pick an IP address to send data when a hostname has several IP addresses.
	// `heartbeat_type udp` is not available with `dns_round_robin true`. Use `heartbeat_type tcp` or `heartbeat_type none`.
	DnsRoundRobin bool `json:"dns_round_robin,omitempty"`
	// Ignore DNS resolution and errors at startup time.
	IgnoreNetworkErrorsAtStartup bool `json:"ignore_network_errors_at_startup,omitempty"`
	// The default version of TLS transport. [TLSv1_1, TLSv1_2] (default: TLSv1_2)
	TlsVersion string `json:"tls_version,omitempty"`
	// The cipher configuration of TLS transport. (default: ALL:!aNULL:!eNULL:!SSLv2)
	TlsCiphers string `json:"tls_ciphers,omitempty"`
	// Skip all verification of certificates or not. (default: false)
	TlsInsecureMode bool `json:"tls_insecure_mode,omitempty"`
	// Allow self signed certificates or not. (default: false)
	TlsAllowSelfSignedCert bool `json:"tls_allow_self_signed_cert,omitempty"`
	// Verify hostname of servers and certificates or not in TLS transport. (default: true)
	TlsVerifyHostname bool `json:"tls_verify_hostname,omitempty"`
	// The additional CA certificate path for TLS.
	TlsCertPath *secret.Secret `json:"tls_cert_path,omitempty"`
	// The client certificate path for TLS
	TlsClientCertPath *secret.Secret `json:"tls_client_cert_path,omitempty"`
	// The client private key path for TLS.
	TlsClientPrivateKeyPath *secret.Secret `json:"tls_client_private_key_path,omitempty"`
	// The client private key passphrase for TLS.
	TlsClientPrivateKeyPassphrase *secret.Secret `json:"tls_client_private_key_passphrase,omitempty"`
	// The certificate thumbprint for searching from Windows system certstore This parameter is for Windows only.
	TlsCertThumbprint string `json:"tls_cert_thumbprint,omitempty"`
	// The certificate logical store name on Windows system certstore. This parameter is for Windows only.
	TlsCertLogicalStoreName string `json:"tls_cert_logical_store_name,omitempty"`
	// Enable to use certificate enterprise store on Windows system certstore. This parameter is for Windows only.
	TlsCertUseEnterpriseStore bool `json:"tls_cert_use_enterprise_store,omitempty"`
	// Enable keepalive connection. (default: false)
	Keepalive bool `json:"keepalive,omitempty"`
	// Expired time of keepalive. Default value is nil, which means to keep connection as long as possible. (default: 0)
	KeepaliveTimeout int `json:"keepalive_timeout,omitempty"`
	// +docLink:"Security,../../common/security/"
	Security *common.Security `json:"security,omitempty"`
	// Verify that a connection can be made with one of out_forward nodes at the time of startup. (default: false)
	VerifyConnectionAtStartup bool `json:"verify_connection_at_startup,omitempty"`
	// +docLink:"Buffer,../buffer/"
	Buffer *Buffer `json:"buffer,omitempty"`
	// The threshold for chunk flush performance check.
	// Parameter type is float, not time, default: 20.0 (seconds)
	// If chunk flush takes longer time than this threshold, fluentd logs warning message and increases metric fluentd_output_status_slow_flush_count.
	SlowFlushLogThreshold string `json:"slow_flush_log_threshold,omitempty"`
}

func (f *ForwardOutput) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	const pluginType = "forward"
	forward := &types.OutputPlugin{
		PluginMeta: types.PluginMeta{
			Type:      pluginType,
			Directive: "match",
			Tag:       "**",
			Id:        id,
		},
	}
	if params, err := types.NewStructToStringMapper(secretLoader).StringsMap(f); err != nil {
		return nil, err
	} else {
		forward.Params = params
	}
	if f.Buffer == nil {
		f.Buffer = &Buffer{}
	}
	if buffer, err := f.Buffer.ToDirective(secretLoader, id); err != nil {
		return nil, err
	} else {
		forward.SubDirectives = append(forward.SubDirectives, buffer)
	}

	if f.Security != nil {
		if format, err := f.Security.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			forward.SubDirectives = append(forward.SubDirectives, format)
		}
	}
	for _, server := range f.FluentdServers {
		if serv, err := server.ToDirective(secretLoader, ""); err != nil {
			return nil, err
		} else {
			forward.SubDirectives = append(forward.SubDirectives, serv)
		}
	}
	return forward, nil
}

// +kubebuilder:object:generate=true
// +docName:"Fluentd Server"
// server
type FluentdServer struct {
	// The IP address or host name of the server.
	Host string `json:"host"`
	// The name of the server. Used for logging and certificate verification in TLS transport (when host is address).
	Name string `json:"name,omitempty"`
	// The port number of the host. Note that both TCP packets (event stream) and UDP packets (heartbeat message) are sent to this port. (default: 24224)
	Port int `json:"port,omitempty"`
	// The shared key per server.
	SharedKey *secret.Secret `json:"shared_key,omitempty"`
	// The username for authentication.
	Username *secret.Secret `json:"username,omitempty"`
	// The password for authentication.
	Password *secret.Secret `json:"password,omitempty"`
	// Marks a node as the standby node for an Active-Standby model between Fluentd nodes. When an active node goes down, the standby node is promoted to an active node. The standby node is not used by the out_forward plugin until then.
	Standby bool `json:"standby,omitempty"`
	// The load balancing weight. If the weight of one server is 20 and the weight of the other server is 30, events are sent in a 2:3 ratio. (default: 60).
	Weight int `json:"weight,omitempty"`
}

func (f *FluentdServer) ToDirective(secretLoader secret.SecretLoader, id string) (types.Directive, error) {
	return types.NewFlatDirective(types.PluginMeta{
		Directive: "server",
	}, f, secretLoader)
}
