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

package firecracker

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/pflag"
)

type Options struct {
	OS                    string
	ImageDir              string
	FirecrackerBinaryPath string

	NumInstances int // 8
	// Number of vCPUs (either 1 or an even number)
	// Required: true
	// Maximum: 32
	// Minimum: 1
	VcpuCount int64
	// Memory size of VM
	// Required: true
	MemSizeMib int64

	GitHubToken string
}

var DefaultOptions = NewOptions()

func NewOptions() *Options {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return &Options{
		OS:                    "focal",
		ImageDir:              "",
		FirecrackerBinaryPath: filepath.Join(dir, "firecracker"),
		NumInstances:          8,
		VcpuCount:             4,
		MemSizeMib:            1024 * 16,
	}
}

func (opts *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&opts.ImageDir, "firecracker.image-dir", opts.ImageDir, "PATH to directory with OS images")
	fs.StringVar(&opts.OS, "firecracker.os", opts.OS, "OS image code")
	fs.StringVar(&opts.FirecrackerBinaryPath, "firecracker.binary-path", opts.FirecrackerBinaryPath, "Path to firecracker binary")

	fs.IntVar(&opts.NumInstances, "firecracker.num-instances", opts.NumInstances, "Number of parallel instances")
	fs.Int64Var(&opts.VcpuCount, "firecracker.vcpu-count", opts.VcpuCount, "Vcpu count for a single instance")
	fs.Int64Var(&opts.MemSizeMib, "firecracker.mem-size-mib", opts.MemSizeMib, "Size(MiB) of memory for a single instance")
}

/*
script gist: https://gist.github.com/tamalsaha/af2f99c80f84410253bd1e532bdfabc7

curl -L https://gist.githubusercontent.com/tamalsaha/af2f99c80f84410253bd1e532bdfabc7/raw/c1b3647e43a28d17d24cbcb2db2063d61bebf455/build_rootfs.sh | bash -s -- bionic 20G

curl -L https://gist.githubusercontent.com/tamalsaha/af2f99c80f84410253bd1e532bdfabc7/raw/c1b3647e43a28d17d24cbcb2db2063d61bebf455/build_rootfs.sh | bash -s -- focal 20G

curl -L https://gist.githubusercontent.com/tamalsaha/af2f99c80f84410253bd1e532bdfabc7/raw/c1b3647e43a28d17d24cbcb2db2063d61bebf455/build_rootfs.sh | bash -s -- jammy 20G
*/
func (opts *Options) RootFSPath() string {
	return filepath.Join(opts.ImageDir, opts.OS, opts.OS+".rootfs")
}

func (opts *Options) KernelImagePath() string {
	return filepath.Join(opts.ImageDir, opts.OS, opts.OS+".vmlinux")
}

func (opts *Options) InitrdPath() string {
	return filepath.Join(opts.ImageDir, opts.OS, opts.OS+".initrd")
}

func WorkflowRunRootFSPath(runID int64) string {
	return fmt.Sprintf("/tmp/%d.rootfs", runID)
}
