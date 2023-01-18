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
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
	"gomodules.xyz/wait"
	"k8s.io/klog/v2"
)

type Options struct {
	AckWait time.Duration

	// same as stream
	Stream string

	// manager id, < 0 means auto detect
	Id int
	// hostname
	Name string

	NumReplicas int
	NumWorkers  int
}

func DefaultOptions() Options {
	hostname, _ := os.Hostname()

	return Options{
		AckWait:     1 * time.Hour,
		Stream:      "ghactions",
		Id:          1,
		Name:        hostname,
		NumReplicas: 1,
		NumWorkers:  runtime.GOMAXPROCS(0),
	}
}

type Manager struct {
	nc      *nats.Conn
	scanSub *nats.Subscription
	ackWait time.Duration

	// same as stream
	stream string

	// manager id, < 0 means auto detect
	id int
	// hostname
	name string

	numReplicas          int
	numWorkersPerReplica int
}

func New(nc *nats.Conn, opts Options) *Manager {
	return &Manager{
		nc:                   nc,
		ackWait:              opts.AckWait,
		stream:               opts.Stream,
		id:                   opts.Id,
		name:                 opts.Name,
		numReplicas:          opts.NumReplicas,
		numWorkersPerReplica: opts.NumWorkers,
	}
}

const (
	JobExecSubject = "ghactions.queue.jobs"
)

func (mgr *Manager) Start(ctx context.Context, jsmOpts ...nats.JSOpt) error {
	jsm, err := mgr.ensureStream(jsmOpts...)
	if err != nil {
		return err
	}

	// create nats consumer
	scanConsumerName := "workers"
	err = mgr.addConsumer(jsm, scanConsumerName)
	if err != nil {
		return err
	}
	scanSubscription, err := jsm.PullSubscribe(JobExecSubject, scanConsumerName, nats.Bind(mgr.stream, scanConsumerName))
	if err != nil {
		return err
	}
	mgr.scanSub = scanSubscription

	// start workers
	klog.Info("Starting workers")
	// Launch two workers to process Foo resources
	for i := 0; i < mgr.numWorkersPerReplica; i++ {
		go wait.Until(mgr.runWorker, 5*time.Second, ctx.Done())
	}

	return nil
}

func (mgr *Manager) ensureStream(jsmOpts ...nats.JSOpt) (nats.JetStreamContext, error) {
	jsm, err := mgr.nc.JetStream(jsmOpts...)
	if err != nil {
		return nil, err
	}

	streamInfo, err := jsm.StreamInfo(mgr.stream, jsmOpts...)

	if streamInfo == nil || err != nil && err.Error() == "nats: stream not found" {
		_, err = jsm.AddStream(&nats.StreamConfig{
			Name:     mgr.stream,
			Subjects: []string{mgr.stream + ".queue.*"},
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

func (mgr *Manager) addConsumer(jsm nats.JetStreamContext, consumerName string) error {
	ackPolicy := nats.AckExplicitPolicy
	_, err := jsm.AddConsumer(mgr.stream, &nats.ConsumerConfig{
		Durable:   consumerName,
		AckPolicy: ackPolicy,
		AckWait:   mgr.ackWait, // TODO: max for any task type
		// The number of pulls that can be outstanding on a pull consumer, pulls received after this is reached are ignored
		MaxWaiting: 1,
		// max working set
		MaxAckPending: mgr.numReplicas * mgr.numWorkersPerReplica,
		// one request per worker
		MaxRequestBatch: 1,
		// max_expires the max amount of time that a pull request with an expires should be allowed to remain active
		// MaxRequestExpires: 1 * time.Second,
		DeliverPolicy: nats.DeliverAllPolicy,
		MaxDeliver:    5,
		FilterSubject: "",
		ReplayPolicy:  nats.ReplayInstantPolicy,
	})
	if err != nil && !strings.Contains(err.Error(), "nats: consumer name already in use") {
		return err
	}
	return nil
}

func (mgr *Manager) runWorker() {
	for {
		err := mgr.processNextMsg()
		if err != nil {
			if !strings.Contains(err.Error(), nats.ErrTimeout.Error()) &&
				!strings.Contains(err.Error(), "nats: Exceeded MaxWaiting") {
				klog.Errorln(err)
			}
			break
		}
	}
}

func (mgr *Manager) processNextMsg() (err error) {
	var msgs []*nats.Msg
	msgs, err = mgr.scanSub.Fetch(1, nats.MaxWait(NatsRequestTimeout))
	if err != nil || len(msgs) == 0 {
		// klog.Error(err)
		// no more msg to process
		err = errors.Wrap(err, "failed to fetch msg")
		return err
	}

	if e2 := ProcessPayload(msgs[0].Data); e2 != nil {
		err = errors.Wrap(e2, "failed to process payload")
	}

	// report failure ?
	if e2 := msgs[0].Ack(); e2 != nil {
		klog.ErrorS(e2, "failed ACK msg")
	}
	return err
}
