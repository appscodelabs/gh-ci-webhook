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
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/appscodelabs/gh-ci-webhook/pkg/providers"

	"github.com/gomodules/agecache"
	"github.com/google/go-github/v68/github"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"
)

const (
	RunnerRegular       = "firecracker"
	RunnerHigh          = "f0"
	RunnerLabelDetector = "label-detector"
)

var (
	actionsBillingCache     *agecache.Cache
	actionsBillingCacheInit sync.Once
)

func initCache(gh *github.Client) {
	actionsBillingCacheInit.Do(func() {
		actionsBillingCache = agecache.New(agecache.Config{
			Capacity: 100,
			MaxAge:   70 * time.Minute,
			MinAge:   60 * time.Minute,
			OnMiss: func(key interface{}) (interface{}, error) {
				return usedUpFreeMinutes(gh, key.(string))
			},
		})
	})
}

func SubmitPayload(gh *github.Client, nc *nats.Conn, r *http.Request, secretToken []byte) error {
	initCache(gh)

	eventType := github.WebHookType(r)
	payload, err := github.ValidatePayload(r, secretToken)
	if err != nil {
		return err
	}
	event, err := github.ParseWebHook(eventType, payload)
	if err != nil {
		return err
	}

	e, ok := event.(*github.WorkflowJobEvent)
	if !ok {
		return nil
	}
	// https://docs.github.com/en/actions/hosting-your-own-runners/autoscaling-with-self-hosted-runners#about-autoscaling
	// BUG: https://github.com/nats-io/natscli/issues/703

	action := e.GetAction()
	label, selfHosted := RunsOnSelfHosted(e)

	var subj string
	if action == "completed" && selfHosted {
		parts := strings.Split(e.GetWorkflowJob().GetRunnerName(), "-")
		hostname := strings.Join(parts[:len(parts)-1], "-")
		subj = fmt.Sprintf("%scompleted.%s", StreamPrefix, hostname)
	} else if action == "queued" && selfHosted {
		subj = fmt.Sprintf("%squeued.%s", StreamPrefix, label)
	}

	if subj != "" {
		var buf bytes.Buffer
		buf.WriteString(eventType)
		buf.WriteRune(':')
		buf.Write(payload)

		js, err := jetstream.New(nc)
		if err != nil {
			return err
		}

		_, err = js.Publish(context.TODO(), subj, buf.Bytes())
		if err != nil {
			return errors.Wrapf(err, "failed to store event %s in NATS", providers.EventKey(e))
		} else {
			klog.Infof("%s: submitted job for %s", subj, providers.EventKey(e))
		}
	}
	return nil
}

func UseRegularRunner(gh *github.Client, org string, private bool) string {
	initCache(gh)

	if private && mustUsedUpFreeMinutes(actionsBillingCache.Get(org)) {
		return RunnerRegular
	}
	return "ubuntu-24.04"
}

func UseHighPriorityRunner(gh *github.Client, org string, private bool) string {
	initCache(gh)

	if private && mustUsedUpFreeMinutes(actionsBillingCache.Get(org)) {
		return RunnerHigh
	}
	return "ubuntu-24.04"
}

func usedUpFreeMinutes(gh *github.Client, org string) (bool, error) {
	ab, _, err := gh.Billing.GetActionsBillingOrg(context.Background(), org)
	if err != nil {
		return false, errors.Wrapf(err, "can't read action billing info for %s", org)
	}
	return ab.IncludedMinutes-ab.TotalMinutesUsed < 60.0, nil
}

func mustUsedUpFreeMinutes(used interface{}, err error) bool {
	if err != nil {
		panic(err)
	}
	return used.(bool)
}

func RunsOnSelfHosted(e *github.WorkflowJobEvent) (string, bool) {
	if len(e.GetWorkflowJob().Labels) != 1 {
		return "", false
	}
	label := e.GetWorkflowJob().Labels[0]
	return label, label == RunnerHigh || label == RunnerRegular
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
	klog.Infof("COMPLETED: %s", providers.EventKey(e))

	return e, mgr.Provider.StopRunner(e)
}
