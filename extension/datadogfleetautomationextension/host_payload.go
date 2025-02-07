package datadogfleetautomationextension

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

type AgentInfo struct {
	APIKey           string        `json:"apiKey"`
	AgentVersion     string        `json:"agentVersion"`
	UUID             string        `json:"uuid"`
	InternalHostname string        `json:"internalHostname"`
	OS               string        `json:"os"`
	AgentFlavor      string        `json:"agent-flavor"`
	Python           string        `json:"python"`
	SystemStats      SystemStats   `json:"systemStats"`
	Meta             Meta          `json:"meta"`
	HostTags         HostTags      `json:"host-tags"`
	ContainerMeta    ContainerMeta `json:"container-meta"`
	Network          interface{}   `json:"network"` // Can be null
	Logs             Logs          `json:"logs"`
	InstallMethod    InstallMethod `json:"install-method"`
	ProxyInfo        ProxyInfo     `json:"proxy-info"`
	OTLP             OTLP          `json:"otlp"`
	Resources        Resources     `json:"resources"`
	Gohai            string        `json:"gohai"`
}

type SystemStats struct {
	CPUCores  int      `json:"cpuCores"`
	Machine   string   `json:"machine"`
	Platform  string   `json:"platform"`
	PythonV   string   `json:"pythonV"`
	Processor string   `json:"processor"`
	MacV      []string `json:"macV"`
	NixV      []string `json:"nixV"`
	FbsdV     []string `json:"fbsdV"`
	WinV      []string `json:"winV"`
}

type Meta struct {
	SocketHostname            string   `json:"socket-hostname"`
	Timezones                 []string `json:"timezones"`
	SocketFqdn                string   `json:"socket-fqdn"`
	EC2Hostname               string   `json:"ec2-hostname"`
	Hostname                  string   `json:"hostname"`
	HostAliases               []string `json:"host_aliases"`
	InstanceID                string   `json:"instance-id"`
	HostnameResolutionVersion int      `json:"hostname-resolution-version"`
}

type HostTags struct {
	System []string `json:"system"`
}

type ContainerMeta struct {
	DockerSwarm   string `json:"docker_swarm"`
	DockerVersion string `json:"docker_version"`
}

type Logs struct {
	Transport                     string `json:"transport"`
	AutoMultiLineDetectionEnabled bool   `json:"auto_multi_line_detection_enabled"`
}

type InstallMethod struct {
	Tool             string `json:"tool"`
	ToolVersion      string `json:"tool_version"`
	InstallerVersion string `json:"installer_version"`
}

type ProxyInfo struct {
	NoProxyNonexactMatch              bool `json:"no-proxy-nonexact-match"`
	ProxyBehaviorChanged              bool `json:"proxy-behavior-changed"`
	NoProxyNonexactMatchExplicitlySet bool `json:"no-proxy-nonexact-match-explicitly-set"`
}

type OTLP struct {
	Enabled bool `json:"enabled"`
}

type Resources struct {
	Meta      ResourcesMeta      `json:"meta"`
	Processes ResourcesProcesses `json:"processes"`
}

type ResourcesMeta struct {
	Host string `json:"host"`
}

type ResourcesProcesses struct {
	Snaps [][]interface{} `json:"snaps"` // Important: Use interface{} for mixed types
}

const sampleHostPayload = `{
  "apiKey": "",
  "agentVersion": "7.69.0",
  "uuid": "",
  "internalHostname": "datadogfleetautomation",
  "os": "darwin",
  "agent-flavor": "collector",
  "python": "3.12.6 (main, Jan 23 2025, 09:32:07) [GCC 12.3.0]",
  "systemStats": {
    "cpuCores": 1,
    "machine": "arm64",
    "platform": "darwin",
    "pythonV": "3.12.6",
    "processor": "",
    "macV": [
      "darwin",
      "15.2",
      ""
    ],
    "nixV": [
      "",
      "",
      ""
    ],
    "fbsdV": [
      "",
      "",
      ""
    ],
    "winV": [
      "",
      "",
      ""
    ]
  },
  "meta": {
    "socket-hostname": "",
    "timezones": [
      "UTC"
    ],
    "socket-fqdn": "",
    "ec2-hostname": "",
    "hostname": "datadogfleetautomation",
    "host_aliases": [],
    "instance-id": "",
    "hostname-resolution-version": 1
  },
  "host-tags": {
    "system": []
  },
  "container-meta": {},
  "network": null,
  "logs": {
    "transport": "",
    "auto_multi_line_detection_enabled": false
  },
  "install-method": {
    "tool": "",
    "tool_version": "",
    "installer_version": ""
  },
  "proxy-info": {
    "no-proxy-nonexact-match": false,
    "proxy-behavior-changed": false,
    "no-proxy-nonexact-match-explicitly-set": false
  },
  "otlp": {
    "enabled": false
  },
  "resources": {
    "meta": {
      "host": "datadogfleetautomation"
    },
    "processes": {
      "snaps": [
        []
      ]
    }
  },
  "gohai": "{\"cpu\":{\"cache_size\":\"0 KB\",\"cache_size_l1\":\"0\",\"cache_size_l2\":\"0\",\"cache_size_l3\":\"0\",\"cpu_cores\":\"16\",\"cpu_logical_processors\":\"16\",\"cpu_pkgs\":\"1\",\"family\":\"none\",\"model\":\"0x000\",\"model_name\":\"0x000\",\"stepping\":\"r0p0\",\"vendor_id\":\"Apple\"},\"filesystem\":[{\"kb_size\":\"65536\",\"mounted_on\":\"/dev/shm\",\"name\":\"shm\"},{\"kb_size\":\"361093644\",\"mounted_on\":\"/etc/hosts\",\"name\":\"/dev/vda1\"},{\"kb_size\":\"1635520\",\"mounted_on\":\"/var/run/docker.sock\",\"name\":\"tmpfs\"},{\"kb_size\":\"8177592\",\"mounted_on\":\"/proc/scsi\",\"name\":\"tmpfs\"},{\"kb_size\":\"8177592\",\"mounted_on\":\"/sys/firmware\",\"name\":\"tmpfs\"},{\"kb_size\":\"361093644\",\"mounted_on\":\"/\",\"name\":\"overlay\"},{\"kb_size\":\"65536\",\"mounted_on\":\"/dev\",\"name\":\"tmpfs\"}],\"memory\":{\"swap_total\":\"1048572kB\",\"total\":\"16747708416\"},\"network\":null,\"platform\":{\"GOOARCH\":\"arm64\",\"GOOS\":\"linux\",\"goV\":\"1.23.3\",\"hardware_platform\":\"aarch64\",\"hostname\":\"45ae4e232445\",\"kernel_name\":\"Linux\",\"kernel_release\":\"6.12.5-linuxkit\",\"kernel_version\":\"#1 SMP Tue Jan 21 10:23:32 UTC 2025\",\"machine\":\"aarch64\",\"os\":\"GNU/Linux\",\"processor\":\"aarch64\"}}"
}`

func (a *AgentInfo) MarshalJSON() ([]byte, error) {
	type agentInfoAlias AgentInfo
	return json.Marshal((*agentInfoAlias)(a))
}

func (a *AgentInfo) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("could not split agent infod payload any more, payload is too big for intake")
}
