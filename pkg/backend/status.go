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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/olekukonko/tablewriter"
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
		ConvertToHumanReadableDateType(ms.Timestamp),
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

func (m *StatusReporter) setStatus(msg []byte) {
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

	m.mu.Lock()
	defer m.mu.Unlock()

	last, found := m.inventory[cur.Name]
	if !found || last.Status != cur.Status {
		m.inventory[cur.Name] = cur
	} else {
		cur.Timestamp = last.Timestamp
		if cur.Comment == "" {
			cur.Comment = last.Comment
		}
		m.inventory[cur.Name] = cur
	}
}

func (m *StatusReporter) Render() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()

	data := make([][]string, 0, len(m.inventory))
	for _, s := range m.inventory {
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
func ConvertToHumanReadableDateType(timestamp time.Time) string {
	if timestamp.IsZero() {
		return "<unknown>"
	}
	var d time.Duration
	now := time.Now()
	if now.After(timestamp) {
		d = now.Sub(timestamp)
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
