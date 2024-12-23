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

package cmds

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/appscodelabs/gh-ci-webhook/pkg/backend"
	"github.com/appscodelabs/gh-ci-webhook/pkg/providers"

	"github.com/google/go-github/v68/github"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
)

func NewCmdWaitForJob() *cobra.Command {
	var (
		ncOpts = backend.NewNATSOptions()
		nc     *nats.Conn
	)
	cmd := &cobra.Command{
		Use:               "wait-for-job",
		Short:             "Wait for Next GitHub Actions runner job",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// ncOpts.Addr = "127.0.0.1:4222"

			hostname, _ := os.Hostname()

			var err error
			nc, err = backend.NewConnection(ncOpts.Addr, ncOpts.CredFile)
			if err != nil {
				return err
			}
			defer nc.Drain() //nolint:errcheck

			var event *github.WorkflowJobEvent
			for {
				event, err = wait_until_job(nc)
				if err != nil {
					klog.ErrorS(err, "error while waiting for next job")
				}
				if event != nil {
					klog.InfoS("tentatively picked job",
						"repo_owner", event.Org.GetLogin(),
						"repo_name", event.Repo.GetName(),
						"workflow_job_id", event.GetWorkflowJob().GetID(),
					)
					break
				}

				backend.ReportStatus(nc, hostname, backend.StatusWaiting)
				time.Sleep(10 * time.Second)
			}

			backend.ReportStatus(nc, hostname, backend.StatusPicked, providers.EventKey(event))

			// https://gist.github.com/tamalsaha/af2f99c80f84410253bd1e532bdfabc7
			// export runner_scope=$(cat repo_owner.txt)
			// export labels=
			label, _ := backend.RunsOnSelfHosted(event)
			jobVars := fmt.Sprintf(`export runner_scope=%s/%s
export labels=%s
`, event.GetRepo().GetOwner().GetLogin(), event.GetRepo().GetName(), label)
			return os.WriteFile("job_vars.txt", []byte(jobVars), 0o644)
		},
	}

	ncOpts.AddFlags(cmd.Flags())

	return cmd
}

// https://natsbyexample.com/examples/jetstream/workqueue-stream/go
func wait_until_job(nc *nats.Conn) (*github.WorkflowJobEvent, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}

	ctx := context.TODO()

	streamName := backend.StreamPrefix + "queued"
	streamQueued, err := js.Stream(ctx, streamName)
	if err != nil {
		return nil, err
	}
	klog.Info("found the stream ", streamName)

	defer printStreamState(ctx, streamQueued)

	payload, err := consumeMsg(ctx, streamQueued, streamName+"."+backend.RunnerHigh)
	if err != nil || len(payload) == 0 {
		payload, err = consumeMsg(ctx, streamQueued, streamName+"."+backend.RunnerRegular)
	}
	if err != nil {
		return nil, err
	} else if len(payload) == 0 {
		return nil, nil
	}

	eventType, payload, found := bytes.Cut(payload, []byte(":"))
	if !found {
		return nil, errors.New("invalid payload format")
	}

	event, err := github.ParseWebHook(string(eventType), payload)
	if err != nil {
		return nil, err
	}
	return event.(*github.WorkflowJobEvent), nil
}

func consumeMsg(ctx context.Context, streamQueued jetstream.Stream, subj string) ([]byte, error) {
	cons, err := streamQueued.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubject: subj,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		err = streamQueued.DeleteConsumer(ctx, cons.CachedInfo().Name)
		if err != nil {
			klog.Errorln(err)
		}
	}()

	/*
			Double-acking is a mechanism used in JetStream to ensure exactly once semantics in message processing.
		    It involves calling the `AckSync()` function instead of `Ack()` to set a reply subject on the Ack and
		    wait for a response from the server on the reception and processing of the acknowledgement. This helps to
		    avoid message duplication and guarantees that the message will not be re-delivered by the consumer.
	*/
	msgs, err := cons.FetchNoWait(1)
	if err != nil {
		return nil, err
	}
	for msg := range msgs.Messages() {
		if err := msg.DoubleAck(ctx); err != nil {
			return nil, err
		} else {
			return msg.Data(), nil // DONE
		}
	}
	if msgs.Error() != nil {
		return nil, errors.Wrap(msgs.Error(), "error during Fetch()")
	}
	return nil, nil
}

func printStreamState(ctx context.Context, stream jetstream.Stream) {
	info, _ := stream.Info(ctx)
	if info != nil {
		b, _ := json.MarshalIndent(info.State, "", " ")
		fmt.Println(string(b))
	}
}
