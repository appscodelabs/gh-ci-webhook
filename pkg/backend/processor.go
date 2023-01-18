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
	"net/url"

	"github.com/google/go-github/v49/github"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"
)

func SubmitPayload(nc *nats.Conn, r *http.Request, secretToken []byte) error {
	eventType := github.WebHookType(r)
	payload, err := github.ValidatePayload(r, secretToken)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.WriteString(eventType)
	buf.WriteRune(':')
	buf.Write(payload)

	_, err = nc.Request(JobExecSubject, buf.Bytes(), NatsRequestTimeout)
	if err != nil {
		return errors.Wrap(err, "failed to store event in NATS")
	}
	return nil
}

func (mgr *Manager) ProcessPayload(slot any, payload []byte) error {
	eventType, payload, found := bytes.Cut(payload, []byte(":"))
	if !found {
		return errors.New("invalid payload format")
	}

	e, err := github.ParseWebHook(string(eventType), payload)
	if err != nil {
		return err
	}

	var query url.Values
	switch event := e.(type) {
	case *github.CheckRunEvent:
		if _, ok := query["pr-repo"]; ok {
			handleCIRepoEvent(event, query)
			return nil
		}
		if _, ok := query["ci-repo"]; ok {
			handlePRRepoEvent(event, query)
			return nil
		}

		return errors.New("unsupported event")
	case *github.PullRequestEvent:
		handlePREvent(event, query)
		return nil
	case *github.IssueCommentEvent:
		handlePRCommentEvent(event, query)
		return nil
	case *github.WorkflowRunEvent:
		fmt.Println("WorkflowRunEvent")
		return nil
	case *github.WorkflowJobEvent:
		// https://docs.github.com/en/actions/hosting-your-own-runners/autoscaling-with-self-hosted-runners#about-autoscaling
		fmt.Println("WorkflowJobEvent")
		s := event.WorkflowJob.GetStatus()
		if s == "queued" {
			StartRunner(event)
			mgr.p.StartRunner(slot, event)
		} else if s == "completed" {
			StopRunner(event)
			mgr.p.StopRunner(slot, event)
		}
		return nil
	default:
		return errors.New("unsupported event")
	}
}

func handlePRCommentEvent(event *github.IssueCommentEvent, query url.Values) {
}

func handlePRRepoEvent(event *github.CheckRunEvent, query url.Values) {
}

func handleCIRepoEvent(event *github.CheckRunEvent, query url.Values) {
}

func handlePREvent(event *github.PullRequestEvent, query url.Values) {
}

func StartRunner(event *github.WorkflowJobEvent) {
	klog.InfoS("start runner", "event", event)
}

func StopRunner(event *github.WorkflowJobEvent) {
	klog.InfoS(">>>>>>>>>>>>>>>> stop runner", "event", event)
}
