package daemon

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/docker/libcontainer/user"

	"github.com/docker/docker/daemon/networkdriver"
	"github.com/docker/docker/opts"
	flag "github.com/docker/docker/pkg/mflag"
	"github.com/docker/docker/pkg/ulimit"
	"github.com/docker/docker/runconfig"
)

const (
	defaultNetworkMtu    = 1500
	disableNetworkBridge = "none"
)

// Config define the configuration of a docker daemon
// These are the configuration settings that you pass
// to the docker daemon when you launch it with say: `docker -d -e lxc`
// FIXME: separate runtime configuration from http api configuration
type Config struct {
	Pidfile                     string
	Root                        string
	AutoRestart                 bool
	Dns                         []string
	DnsSearch                   []string
	EnableIPv6                  bool
	EnableIptables              bool
	EnableIpForward             bool
	EnableIpMasq                bool
	DefaultIp                   net.IP
	BridgeIface                 string
	BridgeIP                    string
	FixedCIDR                   string
	FixedCIDRv6                 string
	InterContainerCommunication bool
	GraphDriver                 string
	GraphOptions                []string
	ExecDriver                  string
	Mtu                         int
	SocketGroup                 string
	EnableCors                  bool
	CorsHeaders                 string
	DisableNetwork              bool
	EnableSelinuxSupport        bool
	Context                     map[string][]string
	TrustKeyPath                string
	Labels                      []string
	Ulimits                     map[string]*ulimit.Ulimit
	LogConfig                   runconfig.LogConfig
	RemappedRoot                string
}

// InstallFlags adds command-line options to the top-level flag parser for
// the current process.
// Subsequent calls to `flag.Parse` will populate config with values parsed
// from the command-line.
func (config *Config) InstallFlags() {
	flag.StringVar(&config.Pidfile, []string{"p", "-pidfile"}, "/var/run/docker.pid", "Path to use for daemon PID file")
	flag.StringVar(&config.Root, []string{"g", "-graph"}, "/var/lib/docker", "Root of the Docker runtime")
	flag.BoolVar(&config.AutoRestart, []string{"#r", "#-restart"}, true, "--restart on the daemon has been deprecated in favor of --restart policies on docker run")
	flag.BoolVar(&config.EnableIptables, []string{"#iptables", "-iptables"}, true, "Enable addition of iptables rules")
	flag.BoolVar(&config.EnableIpForward, []string{"#ip-forward", "-ip-forward"}, true, "Enable net.ipv4.ip_forward")
	flag.BoolVar(&config.EnableIpMasq, []string{"-ip-masq"}, true, "Enable IP masquerading")
	flag.BoolVar(&config.EnableIPv6, []string{"-ipv6"}, false, "Enable IPv6 networking")
	flag.StringVar(&config.BridgeIP, []string{"#bip", "-bip"}, "", "Specify network bridge IP")
	flag.StringVar(&config.BridgeIface, []string{"b", "-bridge"}, "", "Attach containers to a network bridge")
	flag.StringVar(&config.FixedCIDR, []string{"-fixed-cidr"}, "", "IPv4 subnet for fixed IPs")
	flag.StringVar(&config.FixedCIDRv6, []string{"-fixed-cidr-v6"}, "", "IPv6 subnet for fixed IPs")
	flag.BoolVar(&config.InterContainerCommunication, []string{"#icc", "-icc"}, true, "Enable inter-container communication")
	flag.StringVar(&config.GraphDriver, []string{"s", "-storage-driver"}, "", "Storage driver to use")
	flag.StringVar(&config.ExecDriver, []string{"e", "-exec-driver"}, "native", "Exec driver to use")
	flag.BoolVar(&config.EnableSelinuxSupport, []string{"-selinux-enabled"}, false, "Enable selinux support")
	flag.IntVar(&config.Mtu, []string{"#mtu", "-mtu"}, 0, "Set the containers network MTU")
	flag.StringVar(&config.SocketGroup, []string{"G", "-group"}, "docker", "Group for the unix socket")
	flag.BoolVar(&config.EnableCors, []string{"#api-enable-cors", "#-api-enable-cors"}, false, "Enable CORS headers in the remote API, this is deprecated by --api-cors-header")
	flag.StringVar(&config.CorsHeaders, []string{"-api-cors-header"}, "", "Set CORS headers in the remote API")
	opts.IPVar(&config.DefaultIp, []string{"#ip", "-ip"}, "0.0.0.0", "Default IP when binding container ports")
	opts.ListVar(&config.GraphOptions, []string{"-storage-opt"}, "Set storage driver options")
	// FIXME: why the inconsistency between "hosts" and "sockets"?
	opts.IPListVar(&config.Dns, []string{"#dns", "-dns"}, "DNS server to use")
	opts.DnsSearchListVar(&config.DnsSearch, []string{"-dns-search"}, "DNS search domains to use")
	opts.LabelListVar(&config.Labels, []string{"-label"}, "Set key=value labels to the daemon")
	config.Ulimits = make(map[string]*ulimit.Ulimit)
	opts.UlimitMapVar(config.Ulimits, []string{"-default-ulimit"}, "Set default ulimits for containers")
	flag.StringVar(&config.LogConfig.Type, []string{"-log-driver"}, "json-file", "Containers logging driver")
	flag.StringVar(&config.RemappedRoot, []string{"-root"}, "", "User/Group [user|uid[:gid|:group]] for container root")
}

func getDefaultNetworkMtu() int {
	if iface, err := networkdriver.GetDefaultRouteIface(); err == nil {
		return iface.MTU
	}
	return defaultNetworkMtu
}

// Parse the remapped root (user namespace) option, which can be one of:
//   username            - valid username from /etc/passwd
//   username:groupname  - valid username; valid groupname from /etc/group
//   uid                 - 32-bit unsigned int valid Linux UID value
//   uid:gid             - uid value; 32-bit unsigned int Linux GID value
//
//  If no groupname is specified, and a username is specified, an attempt
//  will be made to lookup a gid for that username as a groupname
//
//  If names are used, they are mapped to the appropriate 32-bit unsigned int
func parseRemappedRoot(usergrp string) (int, int, error) {

	var userId, groupId int

	idparts := strings.Split(usergrp, ":")
	if len(idparts) > 2 {
		return 0, 0, fmt.Errorf("Invalid user/group specification in --root: %q", usergrp)
	}

	if uid, err := strconv.ParseInt(idparts[0], 10, 32); err == nil {
		// must be a uid; take it as valid
		userId = int(uid)
		if len(idparts) == 1 {
			// if the uid was numeric and no gid was specified, take the uid as the gid
			groupId = userId
		}
	} else {
		users, err := user.ParsePasswdFileFilter("/etc/passwd", func(usr user.User) bool {
			if usr.Name == idparts[0] {
				return true
			}
			return false
		})
		if err != nil || len(users) == 0 {
			return 0, 0, fmt.Errorf("Unable to find username %q: %v", idparts[0], err)
		}
		userId = users[0].Uid
		if len(idparts) == 1 {
			// we only have a string username, and no group specified; look up gid from username as group
			groups, err := user.ParseGroupFileFilter("/etc/group", func(grp user.Group) bool {
				if grp.Name == idparts[0] {
					return true
				}
				return false
			})
			if err != nil {
				return 0, 0, fmt.Errorf("Error during gid lookup for %q: %v", idparts[0], err)
			}
			if len(groups) == 0 {
				return 0, 0, fmt.Errorf("No such group %q", idparts[0])
			}
			groupId = groups[0].Gid
		}
	}

	if len(idparts) == 2 {
		// groupname or gid is separately specified and must be resolved
		// to a unsigned 32-bit gid
		if gid, err := strconv.ParseInt(idparts[1], 10, 32); err == nil {
			// must be a gid, take it as valid
			groupId = int(gid)
		} else {
			// not a number; attempt a lookup
			groups, err := user.ParseGroupFileFilter("/etc/group", func(grp user.Group) bool {
				if grp.Name == idparts[1] {
					return true
				}
				return false
			})
			if err != nil {
				return 0, 0, fmt.Errorf("Error during gid lookup for %q: %v", idparts[1], err)
			}
			if len(groups) == 0 {
				return 0, 0, fmt.Errorf("No such group %q", idparts[1])
			}
			groupId = groups[0].Gid
		}
	}
	return userId, groupId, nil
}
