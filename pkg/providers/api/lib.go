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

package api

import (
	"fmt"
	"sync"

	"github.com/google/go-github/v55/github"
)

type Interface interface {
	Name() string
	Init() error
	Next() (any, bool)
	Done(any)
	StartRunner(any) error
	StopRunner(*github.WorkflowJobEvent) error
	Status() ([]byte, error)
}

var (
	providers = map[string]Interface{}
	mu        sync.Mutex
)

func Register(p Interface) error {
	mu.Lock()
	defer mu.Unlock()

	_, found := providers[p.Name()]
	if found {
		return fmt.Errorf("provider for %q already registered", p.Name())
	}
	providers[p.Name()] = p
	return nil
}

func MustRegister(p Interface) {
	if err := Register(p); err != nil {
		panic(err)
	}
}

func Provider(name string) (Interface, error) {
	mu.Lock()
	defer mu.Unlock()

	p, ok := providers[name]
	if !ok {
		return nil, fmt.Errorf("provider for %q not registered", name)
	}
	return p, nil
}

func MustProvider(name string) Interface {
	if p, err := Provider(name); err != nil {
		panic(err)
	} else {
		return p
	}
}
