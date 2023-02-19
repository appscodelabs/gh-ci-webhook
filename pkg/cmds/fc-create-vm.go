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
	"os"

	"github.com/appscodelabs/gh-ci-webhook/pkg/providers/api"
	"github.com/appscodelabs/gh-ci-webhook/pkg/providers/firecracker"

	"github.com/spf13/cobra"
	passgen "gomodules.xyz/password-generator"
)

func NewCmdFirecrackerCreateVM() *cobra.Command {
	ghToken := os.Getenv("GITHUB_TOKEN")
	cmd := &cobra.Command{
		Use:               "create-vm",
		Short:             "Firecracker create VM",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			firecracker.DefaultOptions.GitHubToken = ghToken

			p, err := api.Provider("firecracker")
			if err != nil {
				return err
			}
			if err := p.Init(); err != nil {
				return err
			}

			slot := &firecracker.Instance{
				ID:    7,
				UID:   passgen.GenerateForCharset(6, passgen.AlphaNum),
				InUse: false,
			}
			return p.StartRunner(slot, nil)
		},
	}
	cmd.Flags().StringVar(&ghToken, "github-token", ghToken, "GitHub Token")
	firecracker.DefaultOptions.AddFlags(cmd.Flags())

	return cmd
}
