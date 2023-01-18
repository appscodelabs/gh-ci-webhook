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
	"flag"

	"github.com/spf13/cobra"
	"gomodules.xyz/signals"
	v "gomodules.xyz/x/version"
)

func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:               "gh-ci [command]",
		Short:             `gh-ci by AppsCode - GitHub CI for private repos`,
		DisableAutoGenTag: true,
	}

	flags := rootCmd.PersistentFlags()
	flags.AddGoFlagSet(flag.CommandLine)

	ctx := signals.SetupSignalContext()

	rootCmd.AddCommand(NewCmdRun())
	rootCmd.AddCommand(NewCmdHostctl(ctx))
	rootCmd.AddCommand(v.NewCmdVersion())
	return rootCmd
}
