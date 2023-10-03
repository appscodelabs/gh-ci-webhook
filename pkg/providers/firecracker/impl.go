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
	"strconv"
	"strings"

	"github.com/appscodelabs/gh-ci-webhook/pkg/backend"
	"github.com/appscodelabs/gh-ci-webhook/pkg/providers"
	"github.com/appscodelabs/gh-ci-webhook/pkg/providers/api"

	"github.com/google/go-github/v55/github"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
	"gomodules.xyz/go-sh"
	"k8s.io/klog/v2"
)

type impl struct {
	nc  *nats.Conn
	ins *Instances
}

var _ api.Interface = &impl{}

func init() {
	api.MustRegister(&impl{})
}

func (_ impl) Name() string {
	return "firecracker"
}

func (p *impl) Init(nc *nats.Conn) error {
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

func (p impl) StartRunner(slot any) error {
	ins := slot.(*Instance)
	if ins == nil {
		return nil
	}

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	runnerName := fmt.Sprintf("%s-%d", hostname, ins.ID)
	klog.Infoln("Starting VM ", runnerName)
	backend.ReportStatus(p.nc, runnerName, backend.StatusStarting)

	sts, _ := p.Status()
	_ = providers.SendMail(providers.Starting, ins.ID, sts)

	wfRootFSPath := WorkflowRunRootFSPath(ins.UID)
	wfDir := filepath.Dir(wfRootFSPath)
	err = os.MkdirAll(wfDir, 0o755)
	if err != nil {
		return err
	}
	// defer os.RemoveAll(wfDir) // remove in StopRunner

	// optimize rootfs copy
	klog.InfoS("copying rootfs", "path", wfRootFSPath)
	cpfs := fmt.Sprintf("%s-%d", DefaultOptions.RootFSPath(), ins.ID)
	if _, err := os.Stat(cpfs); os.IsNotExist(err) {
		// cp command runs faster than CopyFile
		err = sh.Command("cp", DefaultOptions.RootFSPath(), wfRootFSPath).Run()
		// err = ioutil.CopyFile(wfRootFSPath, DefaultOptions.RootFSPath())
		if err != nil {
			return err
		}
	} else {
		err = os.Rename(cpfs, wfRootFSPath)
		if err != nil {
			return err
		}
	}

	// Setup socket and snapshot + memory paths
	socketPath := filepath.Join(wfDir, fmt.Sprintf("fc-%d", ins.ID))
	fmt.Println("SOCKET_PATH:___", socketPath)

	ctx, cancel := context.WithCancel(context.Background())
	ins.cancel = cancel
	return p.createVM(ctx, ins, runnerName, socketPath)
}

func (p impl) StopRunner(e *github.WorkflowJobEvent) error {
	klog.Infoln("Stopping VM ", e.GetWorkflowJob().GetRunnerName(), "for", providers.EventKey(e))

	backend.ReportStatus(p.nc, e.GetWorkflowJob().GetRunnerName(), backend.StatusStopping, providers.EventKey(e))

	parts := strings.Split(e.GetWorkflowJob().GetRunnerName(), "-")
	instanceID, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return err
	}

	/*
		// optimize rootfs copy
		cpfs := fmt.Sprintf("%s-%d", DefaultOptions.RootFSPath(), instanceID)
		klog.InfoS("copying rootfs", "path", cpfs)
		// cp command runs faster than CopyFile
		err = sh.Command("cp", DefaultOptions.RootFSPath(), cpfs).Run()
		// err = ioutil.CopyFile(cpfs, DefaultOptions.RootFSPath())
		if err != nil {
			return err
		}
	*/

	sts, _ := p.Status()
	_ = providers.SendMail(providers.Shutting, instanceID, sts)

	p.ins.Free(instanceID)

	tap0 := fmt.Sprintf("fc%d", instanceID*4+1)
	tap1 := fmt.Sprintf("fc%d", instanceID*4+2)
	_ = TapDelete(tap0)
	_ = TapDelete(tap1)

	sts2, _ := p.Status()
	_ = providers.SendMail(providers.Shut, instanceID, sts2)

	backend.ReportStatus(p.nc, e.GetWorkflowJob().GetRunnerName(), backend.StatusStopped, providers.EventKey(e))

	return nil
}

func (p impl) Status() ([]byte, error) {
	return json.MarshalIndent(p.ins.slots, "", "  ")
}
