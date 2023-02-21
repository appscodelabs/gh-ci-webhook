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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/appscodelabs/gh-ci-webhook/pkg/providers"
	"github.com/appscodelabs/gh-ci-webhook/pkg/providers/api"

	"github.com/google/go-github/v50/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"golang.org/x/sys/unix"
	"gomodules.xyz/x/ioutil"
)

type impl struct {
	ins *Instances
}

var _ api.Interface = &impl{}

func init() {
	api.MustRegister(&impl{})
}

func (_ impl) Name() string {
	return "firecracker"
}

func (p *impl) Init() error {
	p.ins = NewInstances(DefaultOptions.NumInstances)

	/*
		root@fc-tester:~# ls -l images/focal/
		-rw-r--r-- 1 root root    27613996 Feb 11 09:03 focal.initrd
		-rw-r--r-- 1 root root 21474836480 Feb 11 09:03 focal.rootfs
		-rw-r--r-- 1 root root    49233800 Feb 11 09:03 focal.vmlinux
	*/
	osFiles := []string{
		DefaultOptions.FirecrackerBinaryPath,
		DefaultOptions.RootFSPath(),
		DefaultOptions.KernelImagePath(),
		DefaultOptions.InitrdPath(),
	}
	for _, filename := range osFiles {
		if s, err := os.Stat(filename); err != nil {
			return errors.Wrap(err, "file: "+filename)
		} else if s.Size() == 0 {
			return errors.Errorf("file: %s is empty", filename)
		}
	}

	// Check for kvm and root access
	err := unix.Access("/dev/kvm", unix.W_OK)
	if err != nil {
		return errors.Wrap(err, "file: /dev/kvm")
	}

	if x, y := 0, os.Getuid(); x != y {
		return errors.New("root access denied")
	}
	return nil
}

func (p impl) Next() (any, bool) {
	return p.ins.Next()
}

func (p impl) Done(slot any) {
	ins := slot.(*Instance)
	if ins == nil {
		return
	}
	p.ins.Free(ins.ID)
}

func (p impl) StartRunner(slot any, e *github.WorkflowJobEvent) error {
	ins := slot.(*Instance)
	if ins == nil {
		return nil
	}

	sts, _ := p.Status()
	_ = providers.SendMail(providers.Starting, ins.ID, sts, e)

	wfRootFSPath := WorkflowRunRootFSPath(ins.UID)
	wfDir := filepath.Dir(wfRootFSPath)
	err := os.MkdirAll(wfDir, 0o755)
	if err != nil {
		return err
	}
	// defer os.RemoveAll(wfDir) // remove in StopRunner

	err = ioutil.CopyFile(wfRootFSPath, DefaultOptions.RootFSPath())
	if err != nil {
		return err
	}

	// Setup socket and snapshot + memory paths
	socketPath := filepath.Join(wfDir, fmt.Sprintf("fc-%d", ins.ID))
	fmt.Println("SOCKET_PATH:___", socketPath)

	ctx, cancel := context.WithCancel(context.Background())
	ins.cancel = cancel
	return p.createVM(ctx, ins, socketPath, e)
}

func (p impl) StopRunner(e *github.WorkflowJobEvent) error {
	instanceID, ok := GetSlotForWF(e)
	if !ok {
		return nil
	}

	sts, _ := p.Status()
	_ = providers.SendMail(providers.Shutting, instanceID, sts, e)

	p.ins.Free(instanceID)

	tap0 := fmt.Sprintf("fc%d", instanceID*4+1)
	tap1 := fmt.Sprintf("fc%d", instanceID*4+2)
	_ = TapDelete(tap0)
	_ = TapDelete(tap1)

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	runnerName := fmt.Sprintf("%s-%d", hostname, instanceID)

	// github client
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: DefaultOptions.GitHubToken})
	tc := oauth2.NewClient(ctx, ts)

	client := github.NewClient(tc)
	err = providers.DeleteRunner(ctx, client, e.Repo, runnerName)
	if err != nil {
		return err
	}
	fmt.Println("deleted runner:", runnerName)

	sts2, _ := p.Status()
	_ = providers.SendMail(providers.Shut, instanceID, sts2, e)

	return nil
}

func (p impl) Status() ([]byte, error) {
	return json.MarshalIndent(p.ins.slots, "", "  ")
}
