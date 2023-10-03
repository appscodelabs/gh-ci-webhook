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
	"fmt"
	"os"

	"gomodules.xyz/mailer"
)

type MailType int

const (
	Starting MailType = iota
	Started
	Shutting
	Shut
)

func SendMail(mt MailType, id int, status []byte) error {
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	vmName := fmt.Sprintf("%s-%d", hostname, id)

	var sub string
	switch mt {
	case Starting:
		sub = fmt.Sprintf("Starting VM %s", vmName)
	case Started:
		sub = fmt.Sprintf("Started VM %s", vmName)
	case Shutting:
		sub = fmt.Sprintf("Shutting VM %s", vmName)
	case Shut:
		sub = fmt.Sprintf("Shut down VM %s", vmName)
	}

	var buf bytes.Buffer
	buf.WriteString("**Status:**\n")
	buf.WriteString("```\n")
	buf.Write(status)
	buf.WriteString("\n```\n")

	const email = "tamal+gh-ci-hostctl@appscode.com"
	mm := mailer.Mailer{
		Sender:          email,
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
	return mm.SendMail(mg, email, "", nil)
}
