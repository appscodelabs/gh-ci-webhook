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
	"net/http"
	"os"

	"github.com/appscodelabs/gh-ci-webhook/pkg/backend"
	"github.com/appscodelabs/gh-ci-webhook/pkg/providers/api"
	"github.com/appscodelabs/gh-ci-webhook/pkg/providers/firecracker"
	"github.com/appscodelabs/gh-ci-webhook/pkg/providers/linode"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
)

func NewCmdHostctl(ctx context.Context) *cobra.Command {
	var (
		ghToken = os.Getenv("GITHUB_TOKEN")
		addr    = ":8080"

		opts   = backend.DefaultOptions()
		ncOpts = backend.NewNATSOptions()
		nc     *nats.Conn
	)
	cmd := &cobra.Command{
		Use:               "hostctl",
		Short:             "Run GitHub Actions runner host controller",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			linode.DefaultOptions.GitHubToken = ghToken
			firecracker.DefaultOptions.GitHubToken = ghToken

			// For testing
			// ncOpts.Addr = "192.168.0.233:4222"
			// firecracker.DefaultOptions.NumInstances = 1

			var natsUsername, natsPassword string
			if v, ok := os.LookupEnv("NATS_USERNAME"); ok {
				natsUsername = v
			} else {
				natsUsername = os.Getenv("THIS_IS_NATS_USERNAME")
			}
			if v, ok := os.LookupEnv("NATS_PASSWORD"); ok {
				natsPassword = v
			} else {
				natsPassword = os.Getenv("THIS_IS_NATS_PASSWORD")
			}
			firecracker.DefaultOptions.NatsURL = ncOpts.Addr
			firecracker.DefaultOptions.NatsUsername = natsUsername
			firecracker.DefaultOptions.NatsPassword = natsPassword

			var err error
			nc, err = backend.NewConnection(ncOpts.Addr, ncOpts.CredFile)
			if err != nil {
				return err
			}
			defer nc.Drain() //nolint:errcheck

			mgr := backend.New(nc, opts)
			if err := mgr.Start(ctx); err != nil {
				return err
			}

			go runStatusServer(addr, mgr.Provider)

			<-ctx.Done()
			return nil
		},
	}

	cmd.Flags().StringVar(&ghToken, "github-token", ghToken, "GitHub Token")
	cmd.Flags().StringVar(&addr, "status-server-addr", addr, "host:port of the status server")
	linode.DefaultOptions.AddFlags(cmd.Flags())
	firecracker.DefaultOptions.AddFlags(cmd.Flags())
	opts.AddFlags(cmd.Flags())
	ncOpts.AddFlags(cmd.Flags())

	return cmd
}

func runStatusServer(addr string, p api.Interface) {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Get("/firecracker/status", func(w http.ResponseWriter, r *http.Request) {
		if p != nil {
			data, err := p.Status()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			} else {
				_, _ = w.Write(data)
			}
		}
	})
	klog.Infoln("starting status server", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		klog.Errorln(err)
	}
}
