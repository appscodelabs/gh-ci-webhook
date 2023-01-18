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
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	current "github.com/containernetworking/cni/pkg/types/100"
	sdk "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/firecracker-microvm/firecracker-go-sdk/cni/vmconf"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sys/unix"
	"gomodules.xyz/oneliners"
	"gomodules.xyz/pointer"
)

const (
	// Using default cache directory to ensure collision avoidance on IP allocations
	cniCacheDir = "/var/lib/cni"
	networkName = "fcnet"
	ifName      = "veth0"

	networkMask string = "/24"
	subnet      string = "10.168.0.0" + networkMask

	maxRetries    int           = 100
	backoffTimeMs time.Duration = 500

	MMDS_IP     = "169.254.169.254"
	MMDS_SUBNET = 16

	VMS_NETWORK_PREFIX = "172.26.0"
	VMS_NETWORK_SUBNET = 30
)

func writeCNIConfWithHostLocalSubnet(path, networkName, subnet string) error {
	return os.WriteFile(path, []byte(fmt.Sprintf(`{
		"cniVersion": "0.4.0",
		"name": "%s",
		"plugins": [
		  {
			"type": "ptp",
            "ipMasq": true,
			"ipam": {
			  "type": "host-local",
			  "subnet": "%s",
       	      "resolvConf": "/etc/resolv.conf"
			},
			"dns": {
				"nameservers": [ "1.1.1.1", "8.8.8.8" ]
			}
		  },
		  {
		    "type": "firewall"
		  },
		  {
			"type": "tc-redirect-tap"
		  }
		]
	  }`, networkName, subnet)), 0o644)
}

type configOpt func(*sdk.Config)

func withNetworkInterface(networkInterface sdk.NetworkInterface) configOpt {
	return func(c *sdk.Config) {
		c.NetworkInterfaces = append(c.NetworkInterfaces, networkInterface)
	}
}

func createNewConfig(socketPath string, opts ...configOpt) sdk.Config {
	cpuTemplate := models.CPUTemplate(models.CPUTemplateT2)
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
				PathOnHost:   pointer.StringP(DefaultOptions.RootFSPath()),
			},
		},
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return cfg
}

func connectToVM(m *sdk.Machine, sshKeyPath string) (*ssh.Client, error) {
	key, err := os.ReadFile(sshKeyPath)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	if len(m.Cfg.NetworkInterfaces) == 0 {
		return nil, errors.New("No network interfaces")
	}

	ip := m.Cfg.NetworkInterfaces[len(m.Cfg.NetworkInterfaces)-1].StaticConfiguration.IPConfiguration.IPAddr.IP // IP of VM

	return ssh.Dial("tcp", fmt.Sprintf("%s:22", ip), config)
}

func createSnapshotSSH(ctx context.Context, instanceID int, socketPath string) string {
	//dir, err := os.Getwd()
	//if err != nil {
	//	log.Fatal(err)
	//}

	// cniConfDir := filepath.Join(dir, "cni.conf")
	// cniBinPath := []string{filepath.Join(dir, "bin")} // CNI binaries

	//// Network config
	//cniConfPath := fmt.Sprintf("%s/%s.conflist", cniConfDir, networkName)
	//err = writeCNIConfWithHostLocalSubnet(cniConfPath, networkName, subnet)
	//if err != nil {
	//	log.Fatal(err)
	//}
	//defer os.Remove(cniConfPath)

	//networkInterface := sdk.NetworkInterface{
	//	StaticConfiguration: &sdk.StaticNetworkConfiguration{
	//		MacAddress:      "",
	//		HostDevName:     "",
	//		IPConfiguration: nil,
	//	},
	//	CNIConfiguration: &sdk.CNIConfiguration{
	//		NetworkName: networkName,
	//		IfName:      ifName,
	//		ConfDir:     cniConfDir,
	//		BinPath:     cniBinPath,
	//		VMIfName:    "eth0",
	//	},
	//}

	egressIface, err := GetEgressInterface()
	if err != nil {
		panic(err)
	}
	fmt.Println("EgressInterface:", egressIface)

	// binary.Write(a, binary.LittleEndian, myInt)
	// ip0 := fmt.Sprintf("%s.%d", VMS_NETWORK_PREFIX, (instanceID+1)*2)
	// ip1 := fmt.Sprintf("%s.%d", VMS_NETWORK_PREFIX, (instanceID+1)*2+1)
	ip0 := fmt.Sprintf("%s.%d", VMS_NETWORK_PREFIX, instanceID*4+1)
	ip1 := fmt.Sprintf("%s.%d", VMS_NETWORK_PREFIX, instanceID*4+2)

	fmt.Println("instanceID:", instanceID)
	eth0Mac := MacAddr(net.ParseIP(ip0).To4())
	fmt.Println("ip0:", ip0, eth0Mac)
	eth1Mac := MacAddr(net.ParseIP(ip1).To4())
	fmt.Println("ip1:", ip1, eth1Mac)

	// tap0 := fmt.Sprintf("tap%d", (instanceID+1)*2)
	// tap1 := fmt.Sprintf("tap%d", (instanceID+1)*2+1)
	tap0 := fmt.Sprintf("tap%d", instanceID*4+1)
	tap1 := fmt.Sprintf("tap%d", instanceID*4+2)

	fmt.Println(tap0, tap1)
	// TODO: enable again
	//if _, err := CreateTap(tap0, ""); err != nil {
	//	panic(err)
	//}
	//if _, err := CreateTap(tap1, fmt.Sprintf("%s/%d", ip0, VMS_NETWORK_SUBNET)); err != nil {
	//	panic(err)
	//}
	//if err = SetupIPTables(egressIface, tap1); err != nil {
	//	panic(err)
	//}

	nf0 := sdk.NetworkInterface{
		StaticConfiguration: &sdk.StaticNetworkConfiguration{
			MacAddress:  eth0Mac,
			HostDevName: tap0,
			IPConfiguration: &sdk.IPConfiguration{
				IPAddr:      net.IPNet{IP: net.ParseIP(MMDS_IP), Mask: net.CIDRMask(MMDS_SUBNET, 8*net.IPv4len)},
				Gateway:     nil,
				Nameservers: nil,
				IfName:      "eth0",
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
				IfName:      "eth1",
			},
		},
	}

	socketFile := fmt.Sprintf("%s.create", socketPath)

	cfg := createNewConfig(socketFile, withNetworkInterface(nf0), withNetworkInterface(nf1))
	cfg.MmdsAddress = net.ParseIP(MMDS_IP)
	cfg.MmdsVersion = sdk.MMDSv1

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
		log.Fatal(err)
	}
	defer os.Remove(socketFile)

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

				// http://72.14.182.73:8090/latest/
				ds := fmt.Sprintf("nocloud-net;s=http://%s/latest/", MMDS_IP) // "72.14.182.73:8090")
				kernelArgs["ds"] = &ds

				netcfg, err := BuildNetCfg(eth0Mac, eth1Mac, ip0, ip1)
				if err != nil {
					return err
				}
				kernelArgs["network-config"] = &netcfg

				ipBootParam := func(conf *sdk.IPConfiguration) string {
					// the vmconf package already has a function for doing this, just re-use it
					vmConf := vmconf.StaticNetworkConf{
						// VMNameservers: conf.Nameservers,
						VMIPConfig: &current.IPConfig{
							Address: conf.IPAddr,
							Gateway: conf.Gateway,
						},
						VMIfName: conf.IfName,
					}
					return vmConf.IPBootParam()
				}
				ipBootParam2 := ipBootParam(m.Cfg.NetworkInterfaces[len(m.Cfg.NetworkInterfaces)-1].StaticConfiguration.IPConfiguration)
				// https://linuxlink.timesys.com/docs/static_ip
				// The Ethernet device eth0 will be automatically configured using BOOTP.
				ipBootParam2 = strings.ReplaceAll(ipBootParam2, ":off:", ":bootp:")
				oneliners.FILE("IP:", ipBootParam2)
				// kernelArgs["ip"] = &ipBootParam2

				m.Cfg.KernelArgs = `console=ttyS0 noapic reboot=k panic=1 pci=off nomodules rw ` + kernelArgs.String()

				fmt.Println("KERNEL__:", m.Cfg.KernelArgs)
				fmt.Println("_______________________")

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
	}

	err = m.Start(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := m.StopVMM(); err != nil {
			log.Fatal(err)
		}
	}()
	defer func() {
		if err := m.Shutdown(ctx); err != nil {
			log.Fatal(err)
		}
	}()

	{
		mmds, err := BuildData(instanceID, "tamalsaha")
		if err != nil {
			log.Fatal(err)
		}
		err = m.SetMetadata(ctx, mmds)
		if err != nil {
			log.Fatal(err)
		}
	}

	hostDevName := m.Cfg.NetworkInterfaces[len(m.Cfg.NetworkInterfaces)-1].StaticConfiguration.HostDevName
	fmt.Printf("hostDevName: %v\n", hostDevName)
	vmIP := m.Cfg.NetworkInterfaces[len(m.Cfg.NetworkInterfaces)-1].StaticConfiguration.IPConfiguration.IPAddr.IP.String()
	fmt.Printf("IP of VM: %v\n", vmIP)

	installSignalHandlers(ctx, m)

	// wait for the VMM to exit
	if err := m.Wait(ctx); err != nil {
		log.Errorf("Wait returned an error %s", err)
	}
	log.Printf("Start machine was happy")

	//sshKeyPath := filepath.Join(dir, "root-drive-ssh-key")
	//
	//var client *ssh.Client
	//for i := 0; i < maxRetries; i++ {
	//	client, err = connectToVM(m, sshKeyPath)
	//	if err != nil {
	//		time.Sleep(backoffTimeMs * time.Millisecond)
	//	} else {
	//		break
	//	}
	//}
	//if err != nil {
	//	log.Fatal(err)
	//}
	//defer client.Close()
	//
	//session, err := client.NewSession()
	//if err != nil {
	//	log.Fatal(err)
	//}
	//defer session.Close()
	//
	//fmt.Println(`Sending "sleep 422" command...`)
	//err = session.Start(`sleep 422`)
	//if err != nil {
	//	log.Fatal(err)
	//}

	//time.Sleep(60 * time.Minute)
	//
	//fmt.Println("Creating snapshot...")
	//
	//err = m.PauseVM(ctx)
	//if err != nil {
	//	log.Fatal(err)
	//}
	//
	//err = m.CreateSnapshot(ctx, memPath, snapPath)
	//if err != nil {
	//	log.Fatal(err)
	//}
	//
	//err = m.ResumeVM(ctx)
	//if err != nil {
	//	log.Fatal(err)
	//}

	fmt.Println("Snapshot created")
	return vmIP
}

// Install custom signal handlers:
func installSignalHandlers(ctx context.Context, m *sdk.Machine) {
	go func() {
		// Clear some default handlers installed by the firecracker SDK:
		signal.Reset(os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

		for {
			switch s := <-c; {
			case s == syscall.SIGTERM || s == os.Interrupt:
				log.Printf("Caught signal: %s, requesting clean shutdown", s.String())
				if err := m.Shutdown(ctx); err != nil {
					log.Errorf("An error occurred while shutting down Firecracker VM: %v", err)
				}
			case s == syscall.SIGQUIT:
				log.Printf("Caught signal: %s, forcing shutdown", s.String())
				if err := m.StopVMM(); err != nil {
					log.Errorf("An error occurred while stopping Firecracker VMM: %v", err)
				}
			}
		}
	}()
}

/*
sudo ip tuntap add tap2 mode tap
sudo ip link set tap2 up

sudo ip tuntap add tap3 mode tap
sudo ip addr add 172.26.0.2/31 dev tap3
sudo ip link set tap3 up

sudo sh -c "echo 1 > /proc/sys/net/ipv4/ip_forward"
sudo iptables -t nat -A POSTROUTING -o bond0 -j MASQUERADE
sudo iptables -A FORWARD -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
sudo iptables -A FORWARD -i tap3 -o bond0 -j ACCEPT

$ firectl --kernel=./vmlinux --root-drive=./root-drive-with-ssh.img --kernel-opts="console=ttyS0 noapic reboot=k panic=1 pci=off nomodules rw" --tap-device=tap3/AA:FC:ac:1a:00:03 --tap-device=tap2/AA:FC:ac:1a:00:02

# ip addr add 172.26.0.3/24 dev eth0
# ip link set eth0 up
# ip route add default via 172.26.0.2 dev eth0

# MMDS
ip link set eth1 up
ip route add 169.254.169.254 dev eth1

# --------

# instance = 0
sudo ip tuntap add tap1 mode tap
sudo ip link set tap1 up

sudo ip tuntap add tap2 mode tap
sudo ip addr add 172.26.0.1/30 dev tap2
sudo ip link set tap2 up

sudo sh -c "echo 1 > /proc/sys/net/ipv4/ip_forward"
sudo iptables -t nat -A POSTROUTING -o bond0 -j MASQUERADE
sudo iptables -A FORWARD -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
sudo iptables -A FORWARD -i tap2 -o bond0 -j ACCEPT

# --------
# instance = 1
sudo ip tuntap add tap5 mode tap
sudo ip link set tap5 up

sudo ip tuntap add tap6 mode tap
sudo ip addr add 172.26.0.5/30 dev tap6
sudo ip link set tap6 up

sudo sh -c "echo 1 > /proc/sys/net/ipv4/ip_forward"
sudo iptables -t nat -A POSTROUTING -o bond0 -j MASQUERADE
sudo iptables -A FORWARD -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
sudo iptables -A FORWARD -i tap6 -o bond0 -j ACCEPT
*/
func main() {
	instanceID := flag.Int("instanceID", 0, "Instance ID, starts from 0")
	flag.Parse()

	// Check for kvm and root access
	err := unix.Access("/dev/kvm", unix.W_OK)
	if err != nil {
		log.Fatal(err)
	}

	if x, y := 0, os.Getuid(); x != y {
		log.Fatal("Root acccess denied")
	}

	//cniConfDir := filepath.Join(dir, "cni.conf")
	//err = os.Mkdir(cniConfDir, 0o777)
	//if err != nil {
	//	log.Fatal(err)
	//}
	//defer os.Remove(cniConfDir)

	// Setup socket and snapshot + memory paths
	tempdir, err := os.MkdirTemp("", "FCGoSDKSnapshotExample")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(tempdir)

	socketPath := filepath.Join(tempdir, fmt.Sprintf("fc-%d", instanceID))

	err = os.Mkdir("snapshotssh", 0o777)
	if err != nil && !errors.Is(err, os.ErrExist) {
		log.Fatal(err)
	}
	oneliners.FILE()

	ctx := context.Background()

	fmt.Println("SOCKET_PATH:___", socketPath)
	ipToRestore := createSnapshotSSH(ctx, *instanceID, socketPath)
	fmt.Println(ipToRestore)
}
