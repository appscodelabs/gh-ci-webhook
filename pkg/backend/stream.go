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
	"time"

	"github.com/appscodelabs/gh-ci-webhook/pkg/providers/api"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/pkg/errors"
	"k8s.io/klog/v2"
)

type Manager struct {
	nc              *nats.Conn
	streamQueued    jetstream.Stream
	streamCompleted jetstream.Stream
	ackWait         time.Duration

	// hostname
	name       string
	numWorkers int

	Provider api.Interface
}

func New(nc *nats.Conn, opts Options) *Manager {
	var p api.Interface
	if opts.Provider != "" {
		p = api.MustProvider(opts.Provider)
	}

	return &Manager{
		nc:         nc,
		ackWait:    opts.AckWait,
		name:       opts.Name,
		numWorkers: opts.NumWorkers,
		Provider:   p,
	}
}

func (mgr *Manager) Start(ctx context.Context, jsOpts ...jetstream.JetStreamOpt) error {
	if mgr.Provider != nil {
		err := mgr.Provider.Init()
		if err != nil {
			return errors.Wrap(err, "failed to init provider")
		}
	}

	err := mgr.EnsureStreams(jsOpts...)
	if err != nil {
		return err
	}

	err = mgr.ProcessCompletedJobs()
	if err != nil {
		return err
	}

	mgr.RunVMs()
	return nil
}

func (mgr *Manager) RunVMs() {
	for {
		slot, found := mgr.Provider.Next()
		if !found {
			break
		}
		err := mgr.Provider.StartRunner(slot)
		if err != nil {
			klog.Errorln(err)
		}
	}
}

func (mgr *Manager) ProcessCompletedJobs() error {
	subj := fmt.Sprintf("%scompleted.machines.%s", StreamPrefix, mgr.name)

	err := mgr.streamCompleted.Purge(context.TODO(), jetstream.WithPurgeSubject(subj))
	if err != nil {
		return err
	}

	cons, err := mgr.streamCompleted.CreateOrUpdateConsumer(context.TODO(), jetstream.ConsumerConfig{
		Durable:       mgr.name,
		FilterSubject: subj,
	})
	if err != nil {
		return err
	}

	iter, err := cons.Messages(jetstream.PullMaxMessages(1))
	if err != nil {
		return err
	}
	sem := make(chan struct{}, mgr.numWorkers)

	// PullMaxMessages determines how many messages will be sent to the client in a single pull request
	go func() {
		for {
			sem <- struct{}{}
			go func() {
				defer func() {
					<-sem
				}()
				msg, err := iter.Next()
				if err != nil {
					// handle err
					klog.Errorln(err)
					return
				}
				// fmt.Printf("Processing msg: %s\n", string(msg.Data()))
				_, err = mgr.ProcessCompletedMsg(msg.Data())
				if err != nil {
					klog.Errorln(err)
				}
				msg.DoubleAck(context.TODO())

				if slot, found := mgr.Provider.Next(); found {
					err := mgr.Provider.StartRunner(slot) // Not 1-1 mapping for the VM shut down to restarted
					if err != nil {
						klog.Errorln(err)
					}
				}

				// TODO: restart using the machine/vm
				// msg.Term()
			}()
		}
	}()
	return nil
}

func (mgr *Manager) EnsureStreams(jsOpts ...jetstream.JetStreamOpt) error {
	s1, err := mgr.ensureStream(StreamPrefix+"queued", jsOpts...)
	if err != nil {
		return err
	}
	mgr.streamQueued = s1

	s2, err := mgr.ensureStream(StreamPrefix+"completed", jsOpts...)
	if err != nil {
		return err
	}
	mgr.streamCompleted = s2

	return nil
}

func (mgr *Manager) ensureStream(stream string, jsOpts ...jetstream.JetStreamOpt) (jetstream.Stream, error) {
	js, err := jetstream.New(mgr.nc, jsOpts...)
	if err != nil {
		return nil, err
	}

	s, err := js.Stream(context.TODO(), stream)
	if errors.Is(err, jetstream.ErrStreamNotFound) {
		s, err = js.CreateStream(context.TODO(), jetstream.StreamConfig{
			Name:     stream,
			Subjects: []string{stream + ".>"},
			// https://docs.nats.io/nats-concepts/core-nats/queue#stream-as-a-queue
			Retention:  jetstream.WorkQueuePolicy,
			MaxMsgs:    -1,
			MaxBytes:   -1,
			Discard:    jetstream.DiscardOld,
			MaxAge:     30 * 24 * time.Hour, // 30 days
			MaxMsgSize: 4 * 1024 * 1024,     // 4 MB
			Storage:    jetstream.FileStorage,
			Replicas:   1, // TODO: configure
			Duplicates: time.Hour,
		})
	}
	return s, err
}

/*
func (mgr *Manager) addConsumer(jsm nats.JetStreamContext, status string) error {
	ackPolicy := nats.AckExplicitPolicy
	_, err := jsm.AddConsumer(mgr.streamPrefix, &nats.ConsumerConfig{
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
		FilterSubject: fmt.Sprintf("%s.machines.%s.%d", mgr.streamPrefix, status, mgr.MachineID),
		ReplayPolicy:  nats.ReplayInstantPolicy,
	})
	if err != nil && !strings.Contains(err.Error(), "nats: consumer name already in use") {
		return err
	}
	return nil
}
*/
