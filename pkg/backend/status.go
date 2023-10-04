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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/olekukonko/tablewriter"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/klog/v2"
)

const subStatus = StreamPrefix + "status"

type Status string

const (
	StatusStarting Status = "starting"
	StatusStarted  Status = "started"
	StatusWaiting  Status = "waiting"
	StatusPicked   Status = "picked"
	StatusStopping Status = "stopping"
	StatusStopped  Status = "stopped"
)

type StatusReporter struct {
	mu        sync.Mutex
	inventory map[string]MachineStatus
	nc        *nats.Conn
}

type MachineStatus struct {
	Name      string
	Status    Status
	Timestamp time.Time
	Comment   string
}

func (ms MachineStatus) Strings() []string {
	return []string{
		ms.Name,
		string(ms.Status),
		ConvertToHumanReadableDateType(&ms.Timestamp),
		ms.Comment,
	}
}

func NewStatusReporter(nc *nats.Conn) (*StatusReporter, error) {
	sp := &StatusReporter{
		nc:        nc,
		inventory: map[string]MachineStatus{},
	}
	_, err := nc.Subscribe(subStatus, func(msg *nats.Msg) {
		sp.setStatus(msg.Data)
		_ = msg.Respond([]byte("OK"))
	})
	return sp, err
}

func (sp *StatusReporter) setStatus(msg []byte) {
	fields := strings.SplitN(string(msg), ",", 3)
	if len(fields) < 2 {
		klog.Errorln("bad status report ", string(msg))
		return
	}
	cur := MachineStatus{
		Name:      fields[0],
		Status:    Status(fields[1]),
		Timestamp: time.Now(),
		Comment:   "",
	}
	if len(fields) == 3 {
		cur.Comment = fields[2]
	}

	sp.mu.Lock()
	defer sp.mu.Unlock()

	sp.inventory[cur.Name] = cur
	/*
		last, found := sp.inventory[cur.Name]
		if !found || last.Status != cur.Status {
			sp.inventory[cur.Name] = cur
		} else {
			cur.Timestamp = last.Timestamp
			if cur.Comment == "" {
				cur.Comment = last.Comment
			}
			sp.inventory[cur.Name] = cur
		}
	*/
}

func (sp *StatusReporter) renderRunnerInfo() []byte {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	data := make([][]string, 0, len(sp.inventory))
	for _, s := range sp.inventory {
		data = append(data, s.Strings())
	}
	sort.Slice(data, func(i, j int) bool {
		return data[i][0] < data[j][0]
	})

	var buf bytes.Buffer

	table := tablewriter.NewWriter(&buf)
	table.SetHeader([]string{"Machine", "Status", "Age", "Comment"})
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")
	table.AppendBulk(data) // Add Bulk Data
	table.Render()

	return buf.Bytes()
}

// ConvertToHumanReadableDateType returns the elapsed time since timestamp in
// human-readable approximation.
// ref: https://github.com/kubernetes/apimachinery/blob/v0.21.1/pkg/api/meta/table/table.go#L63-L70
// But works for timestamp before or after now.
func ConvertToHumanReadableDateType(timestamp *time.Time) string {
	if timestamp == nil || timestamp.IsZero() {
		return "<unknown>"
	}
	var d time.Duration
	now := time.Now()
	if now.After(*timestamp) {
		d = now.Sub(*timestamp)
	} else {
		d = timestamp.Sub(now)
	}
	return duration.HumanDuration(d)
}

func ReportStatus(nc *nats.Conn, name string, s Status, comments ...string) {
	fields := []string{name, string(s)}
	if len(comments) > 0 {
		fields = append(fields, comments[0])
	}
	_, err := nc.Request(subStatus, []byte(strings.Join(fields, ",")), NatsRequestTimeout)
	if err != nil {
		klog.Errorln(err)
	}
}

func (sp *StatusReporter) GenerateMarkdownReport() ([]byte, error) {
	var buf bytes.Buffer

	buf.WriteString("## Runners\n\n")
	buf.Write(sp.renderRunnerInfo())
	buf.WriteRune('\n')

	names := []string{
		"gha_queued",
		"gha_completed",
	}
	streams, err := CollectStreamInfo(sp.nc, names)
	if err != nil {
		return nil, err
	}
	buf.WriteString("## Streams\n\n")
	buf.Write(RenderStreamInfo(streams))
	buf.WriteRune('\n')

	for _, name := range names {
		consumers, err := CollectConsumerInfo(sp.nc, name)
		if err != nil {
			return nil, err
		}
		buf.WriteString(fmt.Sprintf("## Consumers for Stream: %s\n\n", name))
		buf.Write(RenderConsumerInfo(consumers))
		buf.WriteRune('\n')
	}
	return buf.Bytes(), nil
}

func (sp *StatusReporter) GenerateHTMLReport() (string, error) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
		),
	)

	data, err := sp.GenerateMarkdownReport()
	if err != nil {
		return "", err
	}

	var bodyHtml bytes.Buffer
	if err := md.Convert(data, &bodyHtml); err != nil {
		return "", err
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta http-equiv="X-UA-Compatible" content="IE=edge">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <meta http-equiv="refresh" content="10">
  </head>
  <body>
  <pre>%s</pre>
  </body>
</html>`, string(data)), nil
}

func CollectStreamInfo(nc *nats.Conn, names []string) ([]*jetstream.StreamInfo, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}

	ctx := context.TODO()
	result := make([]*jetstream.StreamInfo, 0, len(names))
	for _, name := range names {
		if s, err := js.Stream(ctx, name); err != nil {
			return nil, err
		} else {
			info, err := s.Info(ctx)
			if err != nil {
				return nil, err
			}
			if info != nil {
				result = append(result, info)
			}
		}
	}
	return result, nil
}

func RenderStreamInfo(streams []*jetstream.StreamInfo) []byte {
	data := make([][]string, 0, len(streams))
	for _, s := range streams {
		data = append(data, []string{
			s.Config.Name,
			ConvertToHumanReadableDateType(&s.Created),
			strconv.FormatUint(s.State.Msgs, 10),
			ConvertToHumanReadableDateType(&s.State.LastTime),
		})
	}
	sort.Slice(data, func(i, j int) bool {
		return data[i][0] < data[j][0]
	})

	var buf bytes.Buffer

	table := tablewriter.NewWriter(&buf)
	table.SetHeader([]string{"Name", "Created", "Messages", "Last Message"})
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")
	table.AppendBulk(data) // Add Bulk Data
	table.Render()

	return buf.Bytes()
}

func CollectConsumerInfo(nc *nats.Conn, streamName string) ([]*jetstream.ConsumerInfo, error) {
	ctx := context.TODO()
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}

	s, err := js.Stream(ctx, streamName)
	if err != nil {
		return nil, err
	}

	var result []*jetstream.ConsumerInfo

	consumers := s.ListConsumers(ctx)
	for cons := range consumers.Info() {
		result = append(result, cons)
	}
	if consumers.Err() != nil {
		return nil, err
	}
	return result, nil
}

func RenderConsumerInfo(consumers []*jetstream.ConsumerInfo) []byte {
	data := make([][]string, 0, len(consumers))
	for _, s := range consumers {
		data = append(data, []string{
			s.Config.Name,
			s.Config.Durable,
			ConvertToHumanReadableDateType(&s.Created),
			strconv.FormatBool(!s.PushBound),
			s.Config.FilterSubject,
			ConvertToHumanReadableDateType(s.Delivered.Last),
			ConvertToHumanReadableDateType(s.AckFloor.Last),
		})
	}
	sort.Slice(data, func(i, j int) bool {
		return data[i][0] < data[j][0]
	})

	var buf bytes.Buffer

	table := tablewriter.NewWriter(&buf)
	table.SetHeader([]string{"Name", "Durable", "Created", "Pull", "Filter Subject", "Last Delivery", "Last Ack"})
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")
	table.AppendBulk(data) // Add Bulk Data
	table.Render()

	return buf.Bytes()
}
