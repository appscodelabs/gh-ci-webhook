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

package linode

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/appscodelabs/gh-ci-webhook/pkg/backend"
	"github.com/appscodelabs/gh-ci-webhook/pkg/providers/api"

	"github.com/google/go-github/v55/github"
	"github.com/linode/linodego"
	"github.com/nats-io/nats.go"
	"golang.org/x/oauth2"
	passgen "gomodules.xyz/password-generator"
	"gomodules.xyz/pointer"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	RetryInterval = 30 * time.Second
	RetryTimeout  = 3 * time.Minute
)

type impl struct{}

var _ api.Interface = &impl{}

func init() {
	api.MustRegister(&impl{})
}

func (_ impl) Name() string {
	return "linode"
}

func (_ impl) Init(nc *nats.Conn) error {
	return nil
}

func (_ impl) Next() (any, bool) {
	return nil, false
}

func (_ impl) Done(slot any) {}

func (_ impl) Status() ([]byte, error) {
	return nil, nil
}

func (_ impl) StopRunner(e *github.WorkflowJobEvent) error {
	c := NewClient()

	machineName := fmt.Sprintf("%s-%s-%d", e.Org.GetLogin(), e.Repo.GetName(), e.GetWorkflowJob().GetID())

	filter := fmt.Sprintf(`{"label" : "%v"}`, machineName)
	listOpts := &linodego.ListOptions{PageOptions: nil, Filter: filter}

	instances, err := c.ListInstances(context.Background(), listOpts)
	if err != nil {
		panic(err)
	}
	if len(instances) > 1 {
		return fmt.Errorf("multiple linodes found with label %v", machineName)
	} else if len(instances) == 0 {
		return fmt.Errorf("no linode found with label %v", machineName)
	}

	id := instances[0].ID
	err = c.DeleteInstance(context.Background(), id)
	if err != nil {
		return err
	}
	fmt.Println("instance id:", id)

	return nil
}

func (_ impl) StartRunner(_ any) error {
	c := NewClient()

	machineName := fmt.Sprintf("%s%s", backend.StreamPrefix, passgen.GenerateForCharset(6, passgen.AlphaNum))
	fmt.Println(machineName)

	// machineName := "gh-runner-" + passgen.Generate(6)
	id, err := createInstance(c, machineName)
	if err != nil {
		return err
	}
	fmt.Println("instance id:", id)
	return nil
}

func NewClient() *linodego.Client {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: DefaultOptions.Token})

	oauth2Client := &http.Client{
		Transport: &oauth2.Transport{
			Source: tokenSource,
		},
	}

	c := linodego.NewClient(oauth2Client)
	return &c
}

func createInstance(c *linodego.Client, machineName string) (int, error) {
	sshKeys, err := c.ListSSHKeys(context.Background(), &linodego.ListOptions{})
	if err != nil {
		return 0, err
	}
	authorizedKeys := make([]string, 0, len(sshKeys))
	for _, r := range sshKeys {
		authorizedKeys = append(authorizedKeys, r.SSHKey)
	}

	createOpts := linodego.InstanceCreateOptions{
		Label:          machineName,
		Region:         DefaultOptions.Region,
		Type:           DefaultOptions.MachineType,
		RootPass:       DefaultOptions.RootPassword,
		AuthorizedKeys: authorizedKeys,
		StackScriptData: map[string]string{
			"runner_cfg_pat": DefaultOptions.GitHubToken,
			// "runner_owner":   runnerOwner,
			"runner_name": machineName,
		},
		StackScriptID:  DefaultOptions.StackScriptID,
		Image:          DefaultOptions.Image,
		BackupsEnabled: false,
		PrivateIP:      true,
		SwapSize:       pointer.IntP(0),
	}

	instance, err := c.CreateInstance(context.Background(), createOpts)
	if err != nil {
		return 0, err
	}

	if err := waitForStatus(c, instance.ID, linodego.InstanceRunning); err != nil {
		return 0, err
	}

	return instance.ID, nil
}

func waitForStatus(c *linodego.Client, id int, status linodego.InstanceStatus) error {
	attempt := 0
	klog.Infoln("waiting for instance status", "status", status)
	return wait.PollUntilContextTimeout(context.TODO(), RetryInterval, RetryTimeout, true, func(ctx context.Context) (bool, error) {
		attempt++

		instance, err := c.GetInstance(context.Background(), id)
		if err != nil {
			return false, nil
		}
		if instance == nil {
			return false, nil
		}
		klog.Infoln("current instance state", "instance", instance.Label, "status", instance.Status, "attempt", attempt)
		if instance.Status == status {
			klog.Infoln("current instance status", "status", status)
			return true, nil
		}
		return false, nil
	})
}
