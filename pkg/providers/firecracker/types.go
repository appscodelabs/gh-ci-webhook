/*
Copyright AppsCode Inc. and Contributors

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

package firecracker

type NetworkConfig struct {
	Version   int                       `json:"version"`
	Renderer  string                    `json:"renderer,omitempty"`
	Ethernets map[string]EthernetConfig `json:"ethernets"`
}

type EthernetConfig struct {
	Match       EthernetMatcher `json:"match"`
	Addresses   []string        `json:"addresses"`
	Gateway4    string          `json:"gateway4,omitempty"`
	Nameservers *Nameservers    `json:"nameservers,omitempty"`
	Routes      []Route         `json:"routes,omitempty"`
}

type EthernetMatcher struct {
	Macaddress string `json:"macaddress"`
}

type Nameservers struct {
	Addresses []string `json:"addresses"`
}

type Route struct {
	To  string `json:"to"`
	Via string `json:"via"`
}

type Metadata struct {
	InstanceID    string `json:"instance-id"`
	LocalHostname string `json:"local-hostname"`
}

type UserData struct {
	Users   []User   `json:"users"`
	Bootcmd []string `json:"bootcmd"`
}

type User struct {
	Name              string   `json:"name"`
	PlainTextPasswd   string   `json:"plain_text_passwd"`
	Gecos             string   `json:"gecos,omitempty"`
	Shell             string   `json:"shell,omitempty"`
	Groups            string   `json:"groups,omitempty"`
	Sudo              string   `json:"sudo,omitempty"`
	SSHAuthorizedKeys []string `json:"ssh_authorized_keys,omitempty"`
}

type MMDSConfig struct {
	Latest LatestConfig `json:"latest"`
}

type LatestConfig struct {
	MetaData interface{} `json:"meta-data,omitempty"`
	UserData interface{} `json:"user-data,omitempty"`
}
