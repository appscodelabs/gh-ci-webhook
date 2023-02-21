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
	"github.com/appscodelabs/gh-ci-webhook/pkg/providers/firecracker"

	"github.com/spf13/cobra"
)

func NewCmdFirecrackerDeleteTAPDevice() *cobra.Command {
	var device string
	cmd := &cobra.Command{
		Use:               "delete-tap",
		Short:             "Firecracker delete TAP device",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return firecracker.TapDelete(device)
		},
	}
	cmd.Flags().StringVar(&device, "name", device, "Name of TAP device (eg, tap1)")

	return cmd
}
