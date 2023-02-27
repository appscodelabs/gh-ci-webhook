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

package backend

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/appscodelabs/gh-ci-webhook/pkg/providers"

	"github.com/google/go-github/v50/github"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/zeebo/xxh3"
	"gomodules.xyz/sets"
)

func SubmitPayload(nc *nats.Conn, stream string, numMachines uint64, r *http.Request, secretToken []byte) error {
	eventType := github.WebHookType(r)
	payload, err := github.ValidatePayload(r, secretToken)
	if err != nil {
		return err
	}
	e, err := github.ParseWebHook(eventType, payload)
	if err != nil {
		return err
	}

	switch event := e.(type) {
	case *github.WorkflowJobEvent:
		// https://docs.github.com/en/actions/hosting-your-own-runners/autoscaling-with-self-hosted-runners#about-autoscaling
		// BUG: https://github.com/nats-io/natscli/issues/703

		if !sets.NewString(event.GetWorkflowJob().Labels...).Has("self-hosted") {
			return nil
		}

		h := xxh3.New()
		_, _ = h.WriteString(providers.EventKey(event))
		subj := fmt.Sprintf("%s.machines.%s.%d",
			stream,
			event.WorkflowJob.GetStatus(),
			h.Sum64()%numMachines,
		)

		var buf bytes.Buffer
		buf.WriteString(eventType)
		buf.WriteRune(':')
		buf.Write(payload)
		_, err = nc.Request(subj, buf.Bytes(), NatsRequestTimeout)
		if err != nil {
			return errors.Wrap(err, "failed to store event in NATS")
		}
		return nil
	default:
		return errors.New("unsupported event")
	}
}

func (mgr *Manager) ProcessQueuedMsg(slot any, payload []byte) (*github.WorkflowJobEvent, error) {
	eventType, payload, found := bytes.Cut(payload, []byte(":"))
	if !found {
		return nil, errors.New("invalid payload format")
	}

	event, err := github.ParseWebHook(string(eventType), payload)
	if err != nil {
		return nil, err
	}
	e := event.(*github.WorkflowJobEvent)

	if sets.NewString(e.GetWorkflowJob().Labels...).Has("self-hosted") {
		return e, mgr.Provider.StartRunner(slot, e)
	}
	return e, nil
}

func (mgr *Manager) ProcessCompletedMsg(payload []byte) (*github.WorkflowJobEvent, error) {
	eventType, payload, found := bytes.Cut(payload, []byte(":"))
	if !found {
		return nil, errors.New("invalid payload format")
	}

	event, err := github.ParseWebHook(string(eventType), payload)
	if err != nil {
		return nil, err
	}
	e := event.(*github.WorkflowJobEvent)

	if sets.NewString(e.GetWorkflowJob().Labels...).Has("self-hosted") {
		return e, mgr.Provider.StopRunner(e)
	}
	return e, nil
}
