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

package dummy

import (
	"github.com/appscodelabs/gh-ci-webhook/pkg/providers/api"

	"github.com/google/go-github/v52/github"
)

type impl struct{}

var _ api.Interface = &impl{}

func init() {
	api.MustRegister(&impl{})
}

func (_ impl) Name() string {
	return "dummy"
}

func (_ impl) Init() error {
	return nil
}

func (_ impl) Next() (any, bool) {
	return nil, true
}

func (_ impl) Done(slot any) {}

func (_ impl) StopRunner(e *github.WorkflowJobEvent) error {
	return nil
}

func (_ impl) StartRunner(_ any, e *github.WorkflowJobEvent) error {
	return nil
}

func (_ impl) Status() ([]byte, error) {
	return nil, nil
}
