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
	"os"
	"time"

	"github.com/spf13/pflag"
)

type NATSOptions struct {
	Addr     string
	CredFile string
}

func NewNATSOptions() *NATSOptions {
	return &NATSOptions{
		Addr: "this-is-nats.appscode.ninja:4222",
	}
}

func (opts *NATSOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&opts.Addr, "nats-addr", opts.Addr, "NATS serve address")
	fs.StringVar(&opts.CredFile, "nats-credential-file", opts.CredFile, "PATH to NATS credential file")
}

const (
	StreamName = "ghactions"
)

type Options struct {
	AckWait time.Duration

	// same as stream
	Stream string

	// manager id, < 0 means auto detect
	MachineID int
	// hostname
	Name       string
	NumWorkers int

	Provider string
}

func (opts *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&opts.Stream, "stream", opts.Stream, "Name of Jetstream")
	fs.IntVar(&opts.MachineID, "machine-id", opts.MachineID, "Machine ID")
	fs.StringVar(&opts.Provider, "provider", opts.Provider, "Name of runner provider (linode, firecracker)")
}

func DefaultOptions() Options {
	hostname, _ := os.Hostname()

	return Options{
		AckWait:    1 * time.Hour,
		Stream:     StreamName,
		MachineID:  -1,
		Name:       hostname,
		NumWorkers: 1, // MUST be 1
	}
}
