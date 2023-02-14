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

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"
)

func BuildNetCfg(eth0Mac, eth1Mac, ip0, ip1 string) (string, error) {
	/*
		version: 2
		ethernets:
		  eth0:
		    match:
		       macaddress: "AA:FF:00:00:00:01"
		    addresses:
		      - 169.254.0.1/16
		  eth1:
		    match:
		      macaddress: "EE:00:00:00:00:__MAC_OCTET__"
		    addresses:
		      - __INSTANCE_IP__/24
		    gateway4: __GATEWAY__
		    nameservers:
		      addresses: [ 8.8.4.4, 8.8.8.8 ]
	*/

	/*
		# Boot configuration
		instance_ip=$VMS_NETWORK_PREFIX"."$(( $instance_number + 1 ))
		network_config_base64=$( \
			cat conf/cloud-init/network_config.yaml | \
			./tmpl.sh __INSTANCE_IP__ $instance_ip | \
			./tmpl.sh __MAC_OCTET__ $mac_octet | \
			./tmpl.sh __GATEWAY__ $VMS_NETWORK_PREFIX".1" | \
			gzip --stdout - | \
			base64 -w 0
		)
	*/
	nc := NetworkConfig{
		Version:  2,
		Renderer: "networkd",
		Ethernets: map[string]EthernetConfig{
			"eth0": {
				Match: EthernetMatcher{
					Macaddress: eth0Mac,
				},
				Addresses: []string{
					"169.254.0.1/16",
					// fmt.Sprintf("%s/%d", MMDS_IP, MMDS_SUBNET),
				},
				Gateway4:    "",
				Nameservers: nil,
			},
			"eth1": {
				Match: EthernetMatcher{
					Macaddress: eth1Mac, // __MAC_OCTET__
				},
				Addresses: []string{
					fmt.Sprintf("%s/%d", ip1, VMS_NETWORK_SUBNET), // __INSTANCE_IP__/24
				},
				Gateway4: ip0, // __GATEWAY__
				Nameservers: &Nameservers{
					Addresses: []string{
						"1.1.1.1",
						"8.8.8.8",
					},
				},
			},
		},
	}
	ncBytes, err := yaml.Marshal(nc)
	if err != nil {
		return "", err
	}

	fmt.Println(string(ncBytes))

	//var buf bytes.Buffer
	//zw := gzip.NewWriter(&buf)
	////// Setting the Header fields is optional.
	////zw.Name = "a-new-hope.txt"
	////zw.Comment = "an epic space opera by George Lucas"
	////zw.ModTime = time.Date(1977, time.May, 25, 0, 0, 0, 0, time.UTC)
	//
	//_, err = zw.Write(ncBytes)
	//if err != nil {
	//	return "", err
	//}
	//
	//if err := zw.Close(); err != nil {
	//	return "", err
	//}

	cfg := base64.URLEncoding.EncodeToString(ncBytes)
	return cfg, nil
}

func BuildData(instanceID int, ghUsernames ...string) (*MMDSConfig, error) {
	/*
		#cloud-config
		users:
		  - name: fc
		    gecos: Firecracker user
		    shell: /bin/bash
		    groups: sudo
		    sudo: ALL=(ALL) NOPASSWD:ALL
		    ssh_authorized_keys:
		      - __SSH_PUB_KEY__
	*/
	keys, err := getSSHPubKeys(ghUsernames...)
	if err != nil {
		return nil, err
	}
	userData := UserData{
		Users: []User{
			{
				Name: "default",
			},
			{
				Name: "root",
				// PlainTextPasswd: "root",
				Gecos: "Root user",
				Shell: "/bin/bash",
				// Groups:            strings.Join([]string{"sudo", "docker"}, ", "), // groups: users, admin
				// Sudo:              "ALL=(ALL) NOPASSWD:ALL",
				SSHAuthorizedKeys: keys,
			},
			{
				Name:              "runner",
				PlainTextPasswd:   "ubuntu",
				Gecos:             "GitHub Action Runner",
				Shell:             "/bin/bash",
				Groups:            strings.Join([]string{"sudo"}, ", "), // groups: "docker", users, admin
				Sudo:              "ALL=(ALL) NOPASSWD:ALL",
				SSHAuthorizedKeys: keys,
			},
		},
		Bootcmd: []string{
			// BUG: https://bugs.launchpad.net/ubuntu/+source/cloud-initramfs-tools/+bug/1958260
			"apt install --reinstall linux-modules-`uname -r`",
			// "systemctl restart docker",
		},
	}
	//udBytes, err := yaml.Marshal(userData)
	//if err != nil {
	//	return err
	//}

	script := `#!/bin/bash
mkdir test-userscript
touch /test-userscript/userscript.txt
echo "Created by bash shell script" >> /test-userscript/userscript.txt
`

	udBytes, err := PrepareCloudInitUserData(userData, script)
	if err != nil {
		return nil, err
	}
	fmt.Println(string(udBytes))

	md := Metadata{
		InstanceID:    fmt.Sprintf("i-%d", instanceID),
		LocalHostname: "gh-runner",
	}
	mdBytes, err := yaml.Marshal(md)
	if err != nil {
		return nil, err
	}
	fmt.Println(string(mdBytes))

	return &MMDSConfig{
		Latest: LatestConfig{
			MetaData: string(mdBytes),
			UserData: string(udBytes),
		},
	}, nil

	// return json.Marshal(lc)
	//if err != nil {
	//	return err
	//}
	//fmt.Println(string(data))
	//return nil
}

// https://github.com/tamalsaha.keys
func getSSHPubKeys(ghUsernames ...string) ([]string, error) {
	var keys []string
	var buf bytes.Buffer
	for _, username := range ghUsernames {
		resp, err := http.Get(fmt.Sprintf("https://github.com/%s.keys", username))
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		buf.Reset()
		if _, err = io.Copy(&buf, resp.Body); err != nil {
			return nil, err
		}
		userKeys := strings.Split(strings.TrimSpace(buf.String()), "\n")
		keys = append(keys, userKeys...)
	}

	if data, err := os.ReadFile("/root/go/src/github.com/tamalsaha/learn-firecracker/examples/cmd/snapshotting/root-drive-ssh-pubkey"); err == nil {
		keys = append(keys, string(data))
	}

	return keys, nil
}

func PrepareCloudInitUserData(ud UserData, script string) ([]byte, error) {
	// New empty buffer
	body := &bytes.Buffer{}
	// Creates a new multipart Writer with a random boundary
	// writing to the empty buffer
	writer := multipart.NewWriter(body)
	err := writer.SetBoundary(`//`)
	if err != nil {
		return nil, err
	}

	/*
		Content-Type: multipart/mixed; boundary="//"
		MIME-Version: 1.0
	*/

	mpHeader := textproto.MIMEHeader{}
	// Set the Content-Type header
	mpHeader.Set("Content-Type", `multipart/mixed; boundary="//"`)
	mpHeader.Set("MIME-Version", `1.0`)

	{
		keys := make([]string, 0, len(mpHeader))
		for k := range mpHeader {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			for _, v := range mpHeader[k] {
				fmt.Fprintf(body, "%s: %s\r\n", k, v)
			}
		}
		fmt.Fprintf(body, "\r\n")
	}

	udBytes, err := yaml.Marshal(ud)
	if err != nil {
		return nil, err
	}

	// Metadata part
	cloudInitHeader := textproto.MIMEHeader{}
	// Set the Content-Type header
	cloudInitHeader.Set("Content-Type", `text/cloud-config; charset="us-ascii"`)
	cloudInitHeader.Set("MIME-Version", `1.0`)
	cloudInitHeader.Set("Content-Transfer-Encoding", `7bit`)
	cloudInitHeader.Set("Content-Disposition", `attachment; filename="cloud-config.txt"`)

	// Create new multipart part
	cloudInitPart, err := writer.CreatePart(cloudInitHeader)
	if err != nil {
		return nil, err
	}
	// Write the part body
	cloudInitPart.Write([]byte("#cloud-config" + "\n" + string(udBytes)))

	if strings.TrimSpace(script) != "" {
		// Metadata part
		scriptHeader := textproto.MIMEHeader{}
		// Set the Content-Type header
		scriptHeader.Set("Content-Type", `text/x-shellscript; charset="us-ascii"`)
		scriptHeader.Set("MIME-Version", `1.0`)
		scriptHeader.Set("Content-Transfer-Encoding", `7bit`)
		scriptHeader.Set("Content-Disposition", `attachment; filename="userdata.txt"`)

		// Create new multipart part
		scriptPart, err := writer.CreatePart(scriptHeader)
		if err != nil {
			return nil, err
		}
		// Write the part body
		scriptPart.Write([]byte(script))
	}

	// Finish constructing the multipart request body
	writer.Close()

	return body.Bytes(), nil
}
