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

package linode

import (
	"os"

	"github.com/spf13/pflag"
	passgen "gomodules.xyz/password-generator"
)

type Options struct {
	Token         string
	Region        string
	MachineType   string
	Image         string
	StackScriptID int
	RootPassword  string
	GitHubToken   string
}

var DefaultOptions = NewOptions()

func NewOptions() *Options {
	return &Options{
		Token:         os.Getenv("LINODE_CLI_TOKEN"),
		Region:        "us-central",
		MachineType:   "g6-standard-4",
		Image:         "linode/ubuntu20.04",
		StackScriptID: 1018111,
		RootPassword:  passgen.Generate(20),
		GitHubToken:   os.Getenv("GITHUB_TOKEN"),
	}
}

func (opts *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&opts.Token, "linode.token", opts.Token, "Linode api token")
	fs.StringVar(&opts.Region, "linode.region", opts.Region, "Linode machine region")
	fs.StringVar(&opts.MachineType, "linode.machine-type", opts.MachineType, "Linode machine type")
	fs.StringVar(&opts.Image, "linode.image", opts.Image, "Linode image name")
	fs.IntVar(&opts.StackScriptID, "linode.stack-script-id", opts.StackScriptID, "Linode StackScript ID")
	fs.StringVar(&opts.RootPassword, "linode.root-password", opts.RootPassword, "Machine root password")
}
