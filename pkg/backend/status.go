package backend

import (
	"bytes"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/olekukonko/tablewriter"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/klog/v2"
)

type StatusReporter struct {
	mu        sync.Mutex
	inventory map[string]MachineStatus
	nc        *nats.Conn
}

type MachineStatus struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Timestamp time.Time `json:"-"`
}

func NewStatusReporter(nc *nats.Conn) (*StatusReporter, error) {
	sp := &StatusReporter{
		nc:        nc,
		inventory: map[string]MachineStatus{},
	}
	_, err := nc.Subscribe(StreamPrefix+"status", func(msg *nats.Msg) {
		var s MachineStatus
		if err := json.Unmarshal(msg.Data, &s); err != nil {
			klog.ErrorS(err, "failed to parse status")
		} else {
			sp.UpdateStatus(s)
		}
		_ = msg.Respond([]byte("OK"))
	})
	return sp, err
}

func (m *StatusReporter) UpdateStatus(s MachineStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s.Timestamp = time.Now()
	m.inventory[s.Name] = s
}

func (m *StatusReporter) Render() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()

	data := make([][]string, 0, len(m.inventory))
	for _, s := range m.inventory {
		data = append(data, []string{
			s.Name,
			s.Status,
			ConvertToHumanReadableDateType(s.Timestamp),
		})
	}
	sort.Slice(data, func(i, j int) bool {
		return data[i][0] < data[j][0]
	})

	var buf bytes.Buffer

	table := tablewriter.NewWriter(&buf)
	table.SetHeader([]string{"Machine", "Status", "Age"})
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
