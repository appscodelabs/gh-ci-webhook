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
	"context"
	"os"

	"github.com/appscodelabs/gh-ci-webhook/pkg/backend"
	"github.com/appscodelabs/gh-ci-webhook/pkg/providers/firecracker"
	"github.com/appscodelabs/gh-ci-webhook/pkg/providers/linode"

	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"
)

var (
	addr     = "this-is-nats.appscode.ninja:4222"
	credFile string
	nc       *nats.Conn

	ghToken = os.Getenv("GITHUB_TOKEN")

	provider string
)

func NewCmdHostctl(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "hostctl",
		Short:             "Run GitHub Actions runner host controller",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			linode.DefaultOptions.GitHubToken = ghToken
			firecracker.DefaultOptions.GitHubToken = ghToken

			var err error
			nc, err = backend.NewConnection(addr, credFile)
			if err != nil {
				return err
			}
			defer nc.Drain() //nolint:errcheck

			opts := backend.DefaultOptions()
			opts.Provider = provider
			mgr := backend.New(nc, opts)
			if err := mgr.Start(ctx); err != nil {
				return err
			}

			<-ctx.Done()
			return nil
		},
	}

	cmd.Flags().StringVar(&addr, "nats-addr", addr, "NATS serve address")
	cmd.Flags().StringVar(&credFile, "nats-credential-file", credFile, "PATH to NATS credential file")

	cmd.Flags().StringVar(&ghToken, "github-token", ghToken, "GitHub Token")

	cmd.Flags().StringVar(&provider, "provider", provider, "Name of runner provider (linode, firecracker)")
	linode.DefaultOptions.AddFlags(cmd.Flags())
	firecracker.DefaultOptions.AddFlags(cmd.Flags())

	return cmd
}
