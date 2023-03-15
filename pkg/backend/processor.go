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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/appscodelabs/gh-ci-webhook/pkg/providers"

	"github.com/gomodules/agecache"
	"github.com/google/go-github/v50/github"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"github.com/zeebo/xxh3"
	"k8s.io/klog/v2"
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

func SubmitPayload(gh *github.Client, nc *nats.Conn, stream string, numMachines uint64, r *http.Request, secretToken []byte) error {
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
	var subj string
	if action == "completed" && e.GetWorkflowJob().GetRunnerGroupName() == "Default" {
		parts := strings.Split(e.GetWorkflowJob().GetRunnerName(), "-")
		if len(parts) != 3 {
			return fmt.Errorf("invalid runner name %s for %s", e.GetWorkflowJob().GetRunnerName(), providers.EventKey(e))
		}
		machineID, err := strconv.Atoi(parts[1])
		if err != nil {
			return errors.Wrapf(err, "failed to detect machine id from runner name %s for %s", e.GetWorkflowJob().GetRunnerName(), providers.EventKey(e))
		}

		subj = fmt.Sprintf("%s.machines.%s.%d",
			stream,
			e.WorkflowJob.GetStatus(),
			machineID,
		)
	} else if action == "queued" && runsOnSelfHosted(e) {
		//} else if action == "queued" && (runsOnSelfHosted(e) ||
		//	(e.GetRepo().GetPrivate() && mustUsedUpFreeMinutes(actionsBillingCache.Get(e.GetOrg().GetLogin())))) {
		h := xxh3.New()
		_, _ = h.WriteString(providers.EventKey(e))
		subj = fmt.Sprintf("%s.machines.%s.%d",
			stream,
			e.WorkflowJob.GetStatus(),
			h.Sum64()%numMachines,
		)
	}

	if subj != "" {
		var buf bytes.Buffer
		buf.WriteString(eventType)
		buf.WriteRune(':')
		buf.Write(payload)
		_, err = nc.Request(subj, buf.Bytes(), NatsRequestTimeout)
		if err != nil {
			return errors.Wrap(err, "failed to store e in NATS")
		}
		klog.Infoln("submitted job for", providers.EventKey(e))
	}
	return nil
}

func DefaultJobLabel(gh *github.Client, org string, private bool) string {
	initCache(gh)

	if private && mustUsedUpFreeMinutes(actionsBillingCache.Get(org)) {
		return "self-hosted"
	}
	return "ubuntu-20.04"
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

func runsOnSelfHosted(e *github.WorkflowJobEvent) bool {
	return len(e.GetWorkflowJob().Labels) == 1 && e.GetWorkflowJob().Labels[0] == "self-hosted"
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

	return e, mgr.Provider.StartRunner(slot, e)
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

	return e, mgr.Provider.StopRunner(e)
}
