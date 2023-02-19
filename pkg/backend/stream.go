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
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/appscodelabs/gh-ci-webhook/pkg/providers/api"

	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"gomodules.xyz/wait"
	"k8s.io/klog/v2"
)

type Manager struct {
	nc            *nats.Conn
	subsQueued    *nats.Subscription
	subsCompleted *nats.Subscription
	ackWait       time.Duration

	// same as stream
	stream string

	// manager MachineID, < 0 means auto detect
	MachineID int
	// hostname
	name       string
	numWorkers int

	p api.Interface
}

func New(nc *nats.Conn, opts Options) *Manager {
	var p api.Interface
	if opts.Provider != "" {
		p = api.MustProvider(opts.Provider)
	}

	return &Manager{
		nc:         nc,
		ackWait:    opts.AckWait,
		stream:     opts.Stream,
		MachineID:  opts.MachineID,
		name:       opts.Name,
		numWorkers: opts.NumWorkers,
		p:          p,
	}
}

func (mgr *Manager) Start(ctx context.Context, jsmOpts ...nats.JSOpt) error {
	if mgr.p != nil {
		err := mgr.p.Init()
		if err != nil {
			return errors.Wrap(err, "failed to init provider")
		}
	}

	if mgr.MachineID < 0 {
		return errors.New("machine ID is not set")
	}

	jsm, err := mgr.EnsureStream(jsmOpts...)
	if err != nil {
		return err
	}

	// create nats consumer
	{
		status := "queued"
		err = mgr.addConsumer(jsm, status)
		if err != nil {
			return err
		}
		subj := fmt.Sprintf("%s.machines.%s.%d", mgr.stream, status, mgr.MachineID)
		consumerName := fmt.Sprintf("%s-%d", status, mgr.MachineID)
		subsQueued, err := jsm.PullSubscribe(subj, consumerName, nats.Bind(mgr.stream, consumerName))
		if err != nil {
			return err
		}
		mgr.subsQueued = subsQueued
	}

	{
		status := "completed"
		err = mgr.addConsumer(jsm, status)
		if err != nil {
			return err
		}
		subj := fmt.Sprintf("%s.machines.%s.%d", mgr.stream, status, mgr.MachineID)
		consumerName := fmt.Sprintf("%s-%d", status, mgr.MachineID)
		subsCompleted, err := jsm.PullSubscribe(subj, consumerName, nats.Bind(mgr.stream, consumerName))
		if err != nil {
			return err
		}
		mgr.subsCompleted = subsCompleted
	}

	// start workers
	klog.Info("Starting workers")
	for i := 0; i < mgr.numWorkers; i++ {
		go wait.Until(mgr.runQueuedWorker, 5*time.Second, ctx.Done())
	}
	for i := 0; i < mgr.numWorkers; i++ {
		go wait.Until(mgr.runCompletedWorker, 5*time.Second, ctx.Done())
	}

	return nil
}

func (mgr *Manager) EnsureStream(jsmOpts ...nats.JSOpt) (nats.JetStreamContext, error) {
	jsm, err := mgr.nc.JetStream(jsmOpts...)
	if err != nil {
		return nil, err
	}

	streamInfo, err := jsm.StreamInfo(mgr.stream, jsmOpts...)

	if streamInfo == nil || err != nil && err.Error() == "nats: stream not found" {
		_, err = jsm.AddStream(&nats.StreamConfig{
			Name:     mgr.stream,
			Subjects: []string{mgr.stream + ".machines.>"},
			// https://docs.nats.io/nats-concepts/core-nats/queue#stream-as-a-queue
			Retention:  nats.WorkQueuePolicy,
			MaxMsgs:    -1,
			MaxBytes:   -1,
			Discard:    nats.DiscardOld,
			MaxAge:     30 * 24 * time.Hour, // 30 days
			MaxMsgSize: 1 * 1024 * 1024,     // 1 MB
			Storage:    nats.FileStorage,
			Replicas:   1, // TODO: configure
			Duplicates: time.Hour,
		})
		if err != nil {
			return nil, err
		}
	}
	return jsm, nil
}

func (mgr *Manager) addConsumer(jsm nats.JetStreamContext, status string) error {
	ackPolicy := nats.AckExplicitPolicy
	_, err := jsm.AddConsumer(mgr.stream, &nats.ConsumerConfig{
		Durable:   fmt.Sprintf("%s-%d", status, mgr.MachineID),
		AckPolicy: ackPolicy,
		AckWait:   mgr.ackWait, // TODO: max for any task type
		// The number of pulls that can be outstanding on a pull consumer, pulls received after this is reached are ignored
		MaxWaiting: 1,
		// max working set
		MaxAckPending: 2 * mgr.numWorkers, // one for each status
		// one request per worker
		MaxRequestBatch: 1,
		// max_expires the max amount of time that a pull request with an expires should be allowed to remain active
		// MaxRequestExpires: 1 * time.Second,
		DeliverPolicy: nats.DeliverAllPolicy,
		MaxDeliver:    5,
		FilterSubject: fmt.Sprintf("%s.machines.%s.%d", mgr.stream, status, mgr.MachineID),
		ReplayPolicy:  nats.ReplayInstantPolicy,
	})
	if err != nil && !strings.Contains(err.Error(), "nats: consumer name already in use") {
		return err
	}
	return nil
}

func (mgr *Manager) runQueuedWorker() {
	for {
		err := mgr.processNextQueuedMsg()
		if err != nil {
			if !strings.Contains(err.Error(), nats.ErrTimeout.Error()) &&
				!strings.Contains(err.Error(), "nats: Exceeded MaxWaiting") {
				klog.Errorln(err)
			}
			break
		}
	}
}

func (mgr *Manager) processNextQueuedMsg() (err error) {
	slot, found := mgr.p.Next()
	if !found {
		return errors.New("Instance not available")
	}
	defer mgr.p.Done(slot)

	var msgs []*nats.Msg
	msgs, err = mgr.subsQueued.Fetch(1, nats.MaxWait(NatsRequestTimeout))
	if err != nil || len(msgs) == 0 {
		// klog.Error(err)
		// no more msg to process
		err = errors.Wrap(err, "failed to fetch msg")
		return err
	}

	if e2 := mgr.ProcessQueuedMsg(slot, msgs[0].Data); e2 != nil {
		err = errors.Wrap(e2, "failed to process payload")
	}

	// report failure ?
	if e2 := msgs[0].Ack(); e2 != nil {
		klog.ErrorS(e2, "failed ACK msg")
	}
	return err
}

func (mgr *Manager) runCompletedWorker() {
	for {
		err := mgr.processNextCompletedMsg()
		if err != nil {
			if !strings.Contains(err.Error(), nats.ErrTimeout.Error()) &&
				!strings.Contains(err.Error(), "nats: Exceeded MaxWaiting") {
				klog.Errorln(err)
			}
			break
		}
	}
}

func (mgr *Manager) processNextCompletedMsg() (err error) {
	var msgs []*nats.Msg
	msgs, err = mgr.subsCompleted.Fetch(1, nats.MaxWait(NatsRequestTimeout))
	if err != nil || len(msgs) == 0 {
		// klog.Error(err)
		// no more msg to process
		err = errors.Wrap(err, "failed to fetch msg")
		return err
	}

	if e2 := mgr.ProcessCompletedMsg(msgs[0].Data); e2 != nil {
		err = errors.Wrap(e2, "failed to process payload")
	}

	// report failure ?
	if e2 := msgs[0].Ack(); e2 != nil {
		klog.ErrorS(e2, "failed ACK msg")
	}
	return err
}
