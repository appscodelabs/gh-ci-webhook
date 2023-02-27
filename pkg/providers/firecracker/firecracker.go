// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//	http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package firecracker

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/appscodelabs/gh-ci-webhook/pkg/providers"

	sdk "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/google/go-github/v50/github"
	log "github.com/sirupsen/logrus"
	"gomodules.xyz/pointer"
)

const (
	MMDS_IP     = "169.254.169.254"
	MMDS_SUBNET = 16

	VMS_NETWORK_PREFIX = "172.26.0"
	VMS_NETWORK_SUBNET = 30
)

type configOpt func(*sdk.Config)

func withNetworkInterface(networkInterface sdk.NetworkInterface) configOpt {
	return func(c *sdk.Config) {
		c.NetworkInterfaces = append(c.NetworkInterfaces, networkInterface)
	}
}

func createNewConfig(ins *Instance, socketPath string, opts ...configOpt) sdk.Config {
	cpuTemplate := models.CPUTemplateT2
	smt := false

	driveID := "root"
	isRootDevice := true
	isReadOnly := false

	cfg := sdk.Config{
		SocketPath:      socketPath,
		KernelImagePath: DefaultOptions.KernelImagePath(),
		InitrdPath:      DefaultOptions.InitrdPath(),
		MachineCfg: models.MachineConfiguration{
			VcpuCount:   pointer.Int64P(DefaultOptions.VcpuCount),
			CPUTemplate: cpuTemplate,
			MemSizeMib:  pointer.Int64P(DefaultOptions.MemSizeMib),
			Smt:         &smt,
		},
		Drives: []models.Drive{
			{
				DriveID:      &driveID,
				IsRootDevice: &isRootDevice,
				IsReadOnly:   &isReadOnly,
				PathOnHost:   pointer.StringP(WorkflowRunRootFSPath(ins.UID)),
			},
		},
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return cfg
}

func (p impl) createVM(ctx context.Context, ins *Instance, socketPath string, e *github.WorkflowJobEvent) error {
	egressIface, err := GetEgressInterface()
	if err != nil {
		return err
	}
	fmt.Println("EgressInterface:", egressIface)

	ip0 := fmt.Sprintf("%s.%d", VMS_NETWORK_PREFIX, ins.ID*4+1)
	ip1 := fmt.Sprintf("%s.%d", VMS_NETWORK_PREFIX, ins.ID*4+2)

	eth0Mac := MacAddr(net.ParseIP(ip0).To4())
	eth1Mac := MacAddr(net.ParseIP(ip1).To4())
	fmt.Println("instanceID:", ins.ID, "IP:", ip1)

	tap0 := fmt.Sprintf("fc%d", ins.ID*4+1) + ins.UID
	tap1 := fmt.Sprintf("fc%d", ins.ID*4+2) + ins.UID

	if !TapExists(tap0) {
		if _, err := CreateTap(tap0, ""); err != nil {
			return err
		}
	}
	if !TapExists(tap1) {
		if _, err := CreateTap(tap1, fmt.Sprintf("%s/%d", ip0, VMS_NETWORK_SUBNET)); err != nil {
			return err
		}
	}
	if err = SetupIPTables(egressIface, tap1); err != nil {
		return err
	}

	nf0 := sdk.NetworkInterface{
		StaticConfiguration: &sdk.StaticNetworkConfiguration{
			MacAddress:  eth0Mac,
			HostDevName: tap0,
			IPConfiguration: &sdk.IPConfiguration{
				IPAddr:      net.IPNet{IP: net.ParseIP(MMDS_IP), Mask: net.CIDRMask(MMDS_SUBNET, 8*net.IPv4len)},
				Gateway:     nil,
				Nameservers: nil,
				IfName:      "eth0" + ins.UID,
			},
		},
		AllowMMDS: true,
	}
	nf1 := sdk.NetworkInterface{
		StaticConfiguration: &sdk.StaticNetworkConfiguration{
			MacAddress:  eth1Mac,
			HostDevName: tap1,
			IPConfiguration: &sdk.IPConfiguration{
				IPAddr:      net.IPNet{IP: net.ParseIP(ip1), Mask: net.CIDRMask(VMS_NETWORK_SUBNET, 8*net.IPv4len)},
				Gateway:     net.ParseIP(ip0),
				Nameservers: []string{"1.1.1.1", "8.8.8.8"},
				IfName:      "eth1" + ins.UID,
			},
		},
	}

	socketFile := fmt.Sprintf("%s.create", socketPath)

	cfg := createNewConfig(ins, socketFile, withNetworkInterface(nf0), withNetworkInterface(nf1))
	cfg.MmdsAddress = net.ParseIP(MMDS_IP)
	cfg.MmdsVersion = sdk.MMDSv1
	cfg.LogPath = fmt.Sprintf("%s.log", socketPath)
	cfg.LogLevel = "Debug"

	// Use firecracker binary when making machine
	cmd := sdk.VMCommandBuilder{}.
		WithBin(DefaultOptions.FirecrackerBinaryPath).
		WithSocketPath(socketFile).
		WithStdin(os.Stdin).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		Build(ctx)

	m, err := sdk.NewMachine(ctx, cfg, sdk.WithProcessRunner(cmd))
	if err != nil {
		return err
	}
	{
		m.Handlers.FcInit = m.Handlers.FcInit.Swap(sdk.Handler{
			Name: sdk.SetupKernelArgsHandlerName,
			Fn: func(ctx context.Context, m *sdk.Machine) error {
				kernelArgs := parseKernelArgs(m.Cfg.KernelArgs)

				//// If any network interfaces have a static IP configured, we need to set the "ip=" boot param.
				//// Validation that we are not overriding an existing "ip=" setting happens in the network validation
				//if staticIPInterface := m.Cfg.NetworkInterfaces.staticIPInterface(); staticIPInterface != nil {
				//	ipBootParam := staticIPInterface.StaticConfiguration.IPConfiguration.ipBootParam()
				//	kernelArgs["ip"] = &ipBootParam
				//}

				// ds=nocloud-net;s=http://169.254.169.254/latest/
				// network-config=__NETWORK_CONFIG__",

				// cloud-init=disabled
				// disabled := "disabled"
				// kernelArgs["cloud-init"] = &disabled

				ds := fmt.Sprintf("nocloud-net;s=http://%s/latest/", MMDS_IP)
				kernelArgs["ds"] = &ds

				netcfg, err := BuildNetCfg(ins.UID, eth0Mac, eth1Mac, ip0, ip1)
				if err != nil {
					return err
				}
				kernelArgs["network-config"] = &netcfg

				m.Cfg.KernelArgs = `console=ttyS0 noapic reboot=k panic=1 pci=off nomodules rw ` + kernelArgs.String()
				fmt.Println("KERNEL:", m.Cfg.KernelArgs)
				return nil
			},
		})

		// disable network validation
		m.Handlers.Validation = m.Handlers.Validation.Swap(sdk.Handler{
			Name: sdk.ValidateNetworkCfgHandlerName,
			Fn: func(ctx context.Context, m *sdk.Machine) error {
				return nil
			},
		})

		// Set Metadata
		ghRepo := e.GetRepo().GetOwner().GetLogin() + "/" + e.GetRepo().GetName()
		mmds, err := BuildData(ghRepo, DefaultOptions.GitHubToken, ins.ID, "tamalsaha")
		if err != nil {
			return err
		}
		m.Handlers.FcInit = m.Handlers.FcInit.Swappend(sdk.NewSetMetadataHandler(mmds))
	}

	if err := m.Start(ctx); err != nil {
		return err
	}
	SaveWF(ins.ID, e)

	go func() {
		defer func() {
			if err := m.StopVMM(); err != nil {
				log.Errorln(err)
				return
			}
		}()
		defer func() {
			if err := m.Shutdown(ctx); err != nil {
				log.Errorln(err)
				return
			}
		}()

		sts, _ := p.Status()
		_ = providers.SendMail(providers.Started, ins.ID, sts, e)

		// wait for the VMM to exit
		if err := m.Wait(ctx); err != nil {
			log.Errorln(err)
		}
	}()

	return nil
}
