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

package providers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/go-github/v50/github"
	"gomodules.xyz/mailer"
)

type MailType int

const (
	Starting MailType = iota
	Started
	Shutting
	Shut
)

func SendMail(mt MailType, id int, status []byte, e *github.WorkflowJobEvent) error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	vmName := fmt.Sprintf("%s-%d", hostname, id)

	var sub string
	switch mt {
	case Starting:
		sub = fmt.Sprintf("Starting VM %s for %s", vmName, EventKey(e))
	case Started:
		sub = fmt.Sprintf("Started VM %s for %s", vmName, EventKey(e))
	case Shutting:
		sub = fmt.Sprintf("Shutting VM %s for %s", vmName, EventKey(e))
	case Shut:
		sub = fmt.Sprintf("Shut VM %s for %s", vmName, EventKey(e))
	}

	var buf bytes.Buffer
	buf.WriteString("**Status:**\n")
	buf.WriteString("```\n")
	buf.Write(status)
	buf.WriteString("\n```\n")
	buf.WriteString("**Event:**\n")
	buf.WriteString("```\n")
	ed, _ := json.MarshalIndent(e, "", "  ")
	buf.Write(ed)
	buf.WriteString("\n```\n")

	mm := mailer.Mailer{
		Sender:          "tamal+gh-ci-hosttl@appscode.com",
		BCC:             "",
		ReplyTo:         "",
		Subject:         sub,
		Body:            buf.String(),
		Params:          nil,
		AttachmentBytes: nil,
		GDriveFiles:     nil,
	}
	mg, err := mailer.NewSMTPServiceFromEnv()
	if err != nil {
		return err
	}
	return mm.SendMail(mg, "tamal+gh-ci-hosttl@appscode.com", "", nil)
}
