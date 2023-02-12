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
	"os"
	"time"

	"github.com/appscodelabs/gh-ci-webhook/pkg/providers"
	"github.com/appscodelabs/gh-ci-webhook/pkg/providers/api"

	"github.com/google/go-github/v50/github"
	"github.com/linode/linodego"
	"golang.org/x/oauth2"
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

func (_ impl) Init() error {
	return nil
}

func (_ impl) Next() (any, bool) {
	return nil, true
}

func (_ impl) Done(slot any) {}

func (_ impl) StopRunner(_ any, e *github.WorkflowJobEvent) {
	c := NewClient()

	machineName := fmt.Sprintf("%s-%s-%d", e.Org.GetLogin(), e.Repo.GetName(), e.GetWorkflowJob().GetID())

	filter := fmt.Sprintf(`{"label" : "%v"}`, machineName)
	listOpts := &linodego.ListOptions{PageOptions: nil, Filter: filter}

	instances, err := c.ListInstances(context.Background(), listOpts)
	if err != nil {
		panic(err)
	}
	if len(instances) > 1 {
		klog.Errorf("multiple linodes found with label %v", machineName)
		return
	} else if len(instances) == 0 {
		klog.Errorf("no linode found with label %v", machineName)
		return
	}

	id := instances[0].ID
	err = c.DeleteInstance(context.Background(), id)
	if err != nil {
		panic(err)
	}
	fmt.Println("instance id:", id)

	token, found := os.LookupEnv("GITHUB_TOKEN")
	if !found {
		klog.Fatalln("GITHUB_TOKEN env var is not set")
		return
	}

	// github client
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)
	err = providers.DeleteRunner(ctx, client, e.Repo, machineName)
	if err != nil {
		panic(err)
	}
	fmt.Println("deleted machine:", machineName)
}

func (_ impl) StartRunner(_ any, e *github.WorkflowJobEvent) {
	c := NewClient()

	machineName := fmt.Sprintf("%s-%s-%d", e.Org.GetLogin(), e.Repo.GetName(), e.GetWorkflowJob().GetID())
	fmt.Println(machineName)

	// machineName := "gh-runner-" + passgen.Generate(6)
	id, err := createInstance(c, machineName, fmt.Sprintf("%s/%s", e.Org.GetLogin(), e.Repo.GetName()))
	if err != nil {
		panic(err)
	}
	fmt.Println("instance id:", id)
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

func createInstance(c *linodego.Client, machineName, runnerOwner string) (int, error) {
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
			"runner_owner":   runnerOwner,
			"runner_name":    machineName,
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
	return wait.PollImmediate(RetryInterval, RetryTimeout, func() (bool, error) {
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