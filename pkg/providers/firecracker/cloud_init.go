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

// https://gist.github.com/tamalsaha/af2f99c80f84410253bd1e532bdfabc7
func BuildData(ghToken string, instanceID int, ghUsernames ...string) (*MMDSConfig, error) {
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
				Name:              "default",
				SSHAuthorizedKeys: keys,
			},
			{
				Name:              "root",
				Gecos:             "Root user",
				Shell:             "/bin/bash",
				SSHAuthorizedKeys: keys,
			},
			{
				Name:              "runner",
				Gecos:             "GitHub Action Runner",
				Shell:             "/bin/bash",
				Groups:            strings.Join([]string{"sudo"}, ", "),
				Sudo:              "ALL=(ALL) NOPASSWD:ALL",
				SSHAuthorizedKeys: keys,
			},
		},
		// stopped using nomodules in /proc/cmdline
		//Bootcmd: []string{
		//	// BUG: https://bugs.launchpad.net/ubuntu/+source/cloud-initramfs-tools/+bug/1958260
		//	"apt install --reinstall linux-modules-`uname -r`",
		//},
	}

	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	runnerName := fmt.Sprintf("%s-%d", hostname, instanceID)

	//	script := `#!/bin/bash
	//mkdir test-userscript
	//touch /test-userscript/userscript.txt
	//echo "Created by bash shell script" >> /test-userscript/userscript.txt
	//`

	script := fmt.Sprintf(`#! /bin/bash
set -x

# <UDF name="runner_owner" label="GitHub Org or repo" />
# <UDF name="runner_cfg_pat" label="GitHub Personal Token" />
# <UDF name="runner_name" label="Runner Name" />

exec >/root/stackscript.log 2>&1
# http://redsymbol.net/articles/bash-exit-traps/
# https://unix.stackexchange.com/a/308209
function finish {
    result=$?
    [ ! -f /root/result.txt ] && echo $result > /root/result.txt
}
trap finish EXIT
# https://cloud.linode.com/stackscripts/669224
apt-get update
apt upgrade -y
apt remove docker docker-engine docker.io containerd runc
apt-get install -y --no-install-recommends apt-transport-https ca-certificates linux-modules-`+"`uname -r`"+` curl jq gnupg-agent software-properties-common build-essential
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
apt-key fingerprint 0EBFCD88
add-apt-repository \
   "deb [arch=amd64] https://download.docker.com/linux/ubuntu \
   $(lsb_release -cs) \
   stable"
apt update
apt install docker-ce docker-ce-cli containerd.io -y
echo \
  "{
    \"metrics-addr\" : \"0.0.0.0:9323\",
    \"experimental\" : true
  }" > /etc/docker/daemon.json
systemctl restart docker
hostnamectl set-hostname ${RUNNER_NAME}
echo 127.0.1.1 $HOSTNAME.localdomain ${RUNNER_NAME} >> /etc/hosts

chmod a+w /usr/local/bin

# bypass docker hub rate limits
docker login -u tigerworks -p dckr_pat_TQSHB3Z8CoNU8G4jtW7xXOxMefM

# Prepare GitHun Runner user
export USER=runner
# https://docs.docker.com/engine/install/linux-postinstall/
sudo usermod -aG docker $USER
newgrp docker
rsync --archive --chown=$USER:$USER ~/.docker /home/$USER

# Install GitHub Runner
su $USER
cd /home/$USER

export NATS_URL=%s
export NATS_USERNAME=%s
export NATS_PASSWORD=%s

# export RUNNER_OWNER=$(cat repo_owner.txt)
export RUNNER_CFG_PAT=%s
export RUNNER_NAME=%s

# https://github.com/actions/runner/blob/main/docs/automate.md
# https://github.com/actions/actions-runner-controller/issues/84#issuecomment-756971038
# -l ubuntu-latest,ubuntu-20.04
# curl -s https://raw.githubusercontent.com/actions/runner/main/scripts/create-latest-svc.sh | bash -s -- -s ${RUNNER_OWNER} -n ${RUNNER_NAME} -f
# ephemeral runner: https://docs.github.com/en/actions/hosting-your-own-runners/autoscaling-with-self-hosted-runners#using-ephemeral-runners-for-autoscaling
# https://github.blog/changelog/2021-09-20-github-actions-ephemeral-self-hosted-runners-new-webhooks-for-auto-scaling/
curl -fsSL https://gist.githubusercontent.com/tamalsaha/af2f99c80f84410253bd1e532bdfabc7/raw/a3fa2a82ae14d406fd934b90f2be7dc2945607a8/start-runner.sh | bash -s -- -n ${RUNNER_NAME} -f
`, DefaultOptions.NatsURL, DefaultOptions.NatsUsername, DefaultOptions.NatsPassword, ghToken, runnerName)

	udBytes, err := PrepareCloudInitUserData(userData, script)
	if err != nil {
		return nil, err
	}
	fmt.Println(string(udBytes))

	md := Metadata{
		InstanceID:    runnerName, // fmt.Sprintf("i-%d", instanceID),
		LocalHostname: runnerName, // "gh-runner",
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
				_, _ = fmt.Fprintf(body, "%s: %s\r\n", k, v)
			}
		}
		_, _ = fmt.Fprintf(body, "\r\n")
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
	_, _ = cloudInitPart.Write([]byte("#cloud-config" + "\n" + string(udBytes)))

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
		_, _ = scriptPart.Write([]byte(script))
	}

	// Finish constructing the multipart request body
	err = writer.Close()
	if err != nil {
		return nil, err
	}

	return body.Bytes(), nil
}
