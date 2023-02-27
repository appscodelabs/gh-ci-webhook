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
	"strings"
	"syscall"

	"github.com/coreos/go-iptables/iptables"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
	"gomodules.xyz/go-sh"
)

func GetEgressInterface() (string, error) {
	egressIface, err := sh.
		Command("ip", "route", "get", "1.1.1.1").
		Command("grep", "uid").
		Command("sed", `s/.* dev \([^ ]*\) .*/\1/`).
		Output()
	return strings.TrimSpace(string(egressIface)), err
}

// Host iface bond0
// detect using
// EGRESS_IFACE=`ip route get 8.8.8.8 |grep uid |sed 's/.* dev \([^ ]*\) .*/\1/'`
func SetupIPTables(iface, tapDev string) error {
	/*
		sudo sh -c "echo 1 > /proc/sys/net/ipv4/ip_forward"
		sudo iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
		sudo iptables -A FORWARD -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
		sudo iptables -A FORWARD -i tap0 -o eth0 -j ACCEPT
	*/

	// sudo sysctl -w net.ipv4.ip_forward=1 > /dev/null
	filename := "/proc/sys/net/ipv4/ip_forward"
	if err := os.WriteFile(filename, []byte("1"), 0o644); err != nil {
		return errors.Wrapf(err, "failed to write file: %s", filename)
	}

	tbl, err := iptables.New(iptables.IPFamily(iptables.ProtocolIPv4), iptables.Timeout(5))
	if err != nil {
		return errors.Wrap(err, "failed to construct iptables obj")
	}
	err = tbl.AppendUnique("nat", "POSTROUTING", "-o", iface, "-j", "MASQUERADE")
	if err != nil {
		return errors.Wrap(err, "failed to exec: "+"iptables -t nat -A POSTROUTING -o "+iface+" -j MASQUERADE")
	}

	/*
		sudo iptables -A FORWARD -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
		sudo iptables -A FORWARD -i tap0 -o eth0 -j ACCEPT
	*/
	err = tbl.AppendUnique("filter", "FORWARD", "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT")
	if err != nil {
		return errors.Wrap(err, "failed to exec: "+"iptables -A FORWARD -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT")
	}
	err = tbl.AppendUnique("filter", "FORWARD", "-i", tapDev, "-o", iface, "-j", "ACCEPT")
	if err != nil {
		return errors.Wrapf(err, "failed to exec: "+"-i %s -o %s -j ACCEPT", tapDev, iface)
	}

	return nil
}

/*
sudo ip tuntap add tap0 mode tap
sudo ip addr add 172.16.0.1/24 dev tap0
sudo ip link set tap0 up
*/
func CreateTap_(name, cidr string) (netlink.Link, error) {
	tapLinkAttrs := netlink.NewLinkAttrs()
	tapLinkAttrs.Name = name
	tapLink := &netlink.Tuntap{
		LinkAttrs: tapLinkAttrs,

		// We want a tap device (L2) as opposed to a tun (L3)
		Mode: netlink.TUNTAP_MODE_TAP,

		// Firecracker does not support multiqueue tap devices at this time:
		// https://github.com/firecracker-microvm/firecracker/issues/750
		Queues: 1,

		Flags: netlink.TUNTAP_ONE_QUEUE | // single queue tap device
			netlink.TUNTAP_VNET_HDR, // parse vnet headers added by the vm's virtio_net implementation
	}

	err := netlink.LinkAdd(tapLink)
	if err != nil {
		return nil, fmt.Errorf("failed to create tap device: %w", err)
	}

	//for _, tapFd := range tapLink.Fds {
	//	err = unix.IoctlSetInt(int(tapFd.Fd()), unix.TUNSETOWNER, ownerUID)
	//	if err != nil {
	//		return nil, fmt.Errorf("failed to set tap %s owner to uid %d: %w",
	//			name, ownerUID, err)
	//
	//	}
	//
	//	err = unix.IoctlSetInt(int(tapFd.Fd()), unix.TUNSETGROUP, ownerGID)
	//	if err != nil {
	//		return nil, fmt.Errorf("failed to set tap %s group to gid %d: %w",
	//			name, ownerGID, err)
	//
	//	}
	//}

	//err = netlink.LinkSetMTU(tapLink, mtu)
	//if err != nil {
	//	return nil, fmt.Errorf("failed to set tap device MTU to %d: %w", mtu, err)
	//}

	if cidr != "" {
		addr, err := netlink.ParseAddr(cidr) // "172.20.0.1/24"
		if err != nil {
			return nil, err
		}
		err = netlink.AddrAdd(tapLink, addr)
		if err != nil && !errors.Is(err, syscall.EEXIST) {
			// oneliners.FILE(err)
			// fmt.Println("errors.Is(err, syscall.EEXIST) = ", errors.Is(err, syscall.EEXIST))
			// fmt.Printf("%T\n", err)
			return nil, err
		}
	}

	err = netlink.LinkSetUp(tapLink)
	if err != nil {
		return nil, fmt.Errorf("failed to set tap up: %w", err)
	}

	return tapLink, nil
}

/*
sudo ip tuntap add tap0 mode tap
sudo ip addr add 172.16.0.1/24 dev tap0
sudo ip link set tap0 up
*/
func CreateTap(name, cidr string) error {
	if err := sh.Command("ip", "tuntap", "add", name, "mode", "tap").Run(); err != nil {
		return err
	}
	if cidr != "" {
		if err := sh.Command("ip", "addr", "add", cidr, "dev", name).Run(); err != nil {
			return err
		}
	}
	if err := sh.Command("ip", "link", "set", name, "up").Run(); err != nil {
		return err
	}
	return nil
}

func TapExists(name string) bool {
	l, err := netlink.LinkByName(name)
	return l != nil && err == nil
}

func TapDelete_(name string) error {
	if l, err := netlink.LinkByName(name); err == nil {
		return netlink.LinkDel(l)
	} else {
		if _, ok := err.(netlink.LinkNotFoundError); !ok {
			return err
		}
	}
	return nil
}

// sudo ip link del tap0
func TapDelete(name string) error {
	return sh.Command("ip", "link", "del", name).Run()
}

const hexDigit = "0123456789abcdef"

func MacAddr(b []byte) string {
	s := make([]byte, len(b)*3)
	for i, tn := range b {
		s[i*3], s[i*3+1], s[i*3+2] = ':', hexDigit[tn>>4], hexDigit[tn&0xf]
	}
	return "AA:FC" + string(s)
}
