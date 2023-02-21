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
	"os"
	"path/filepath"
	"sync"

	"github.com/appscodelabs/gh-ci-webhook/pkg/providers"

	"github.com/google/go-github/v50/github"
	passgen "gomodules.xyz/password-generator"
)

type Instance struct {
	ID     int
	UID    string
	InUse  bool
	cancel func()
}

func (i *Instance) Free() {
	wfRootFSPath := WorkflowRunRootFSPath(i.UID)
	wfDir := filepath.Dir(wfRootFSPath)
	_ = os.RemoveAll(wfDir)

	i.UID = ""
	i.InUse = false
	if i.cancel != nil {
		i.cancel()
		i.cancel = nil
	}
}

type Instances struct {
	slots []Instance
	mu    sync.Mutex
}

func NewInstances(numInstances int) *Instances {
	out := Instances{
		slots: make([]Instance, numInstances),
	}
	for i := 0; i < numInstances; i++ {
		out.slots[i].ID = i
	}
	return &out
}

func (i *Instances) Next() (*Instance, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()

	for id, slot := range i.slots {
		if !slot.InUse {
			i.slots[id].UID = passgen.GenerateForCharset(6, passgen.AlphaNum)
			i.slots[id].InUse = true
			return &i.slots[id], true
		}
	}
	return nil, false
}

func (i *Instances) Free(id int) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.slots[id].InUse {
		i.slots[id].Free()
	}
}

var (
	wfToInstanceID = map[string]int{}
	muWF           sync.Mutex
)

func SaveWF(id int, e *github.WorkflowJobEvent) {
	muWF.Lock()
	defer muWF.Unlock()
	wfToInstanceID[providers.EventKey(e)] = id
}

func GetSlotForWF(e *github.WorkflowJobEvent) (int, bool) {
	muWF.Lock()
	defer muWF.Unlock()
	id, ok := wfToInstanceID[providers.EventKey(e)]
	return id, ok
}
