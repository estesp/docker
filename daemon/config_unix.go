// +build linux freebsd

package daemon

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/opencontainers/runc/libcontainer/user"

	"github.com/docker/docker/opts"
	flag "github.com/docker/docker/pkg/mflag"
	"github.com/docker/docker/pkg/ulimit"
)

var (
	defaultPidFile = "/var/run/docker.pid"
	defaultGraph   = "/var/lib/docker"
	defaultExec    = "native"
)

// Config defines the configuration of a docker daemon.
// These are the configuration settings that you pass
// to the docker daemon when you launch it with say: `docker daemon -e lxc`
type Config struct {
	CommonConfig

	// Fields below here are platform specific.

	CorsHeaders          string
	EnableCors           bool
	EnableSelinuxSupport bool
	RemappedRoot         string
	SocketGroup          string
	Ulimits              map[string]*ulimit.Ulimit
}

// bridgeConfig stores all the bridge driver specific
// configuration.
type bridgeConfig struct {
	EnableIPv6                  bool
	EnableIPTables              bool
	EnableIPForward             bool
	EnableIPMasq                bool
	EnableUserlandProxy         bool
	DefaultIP                   net.IP
	Iface                       string
	IP                          string
	FixedCIDR                   string
	FixedCIDRv6                 string
	DefaultGatewayIPv4          net.IP
	DefaultGatewayIPv6          net.IP
	InterContainerCommunication bool
}

// InstallFlags adds command-line options to the top-level flag parser for
// the current process.
// Subsequent calls to `flag.Parse` will populate config with values parsed
// from the command-line.
func (config *Config) InstallFlags(cmd *flag.FlagSet, usageFn func(string) string) {
	// First handle install flags which are consistent cross-platform
	config.InstallCommonFlags(cmd, usageFn)

	// Then platform-specific install flags
	cmd.BoolVar(&config.EnableSelinuxSupport, []string{"-selinux-enabled"}, false, usageFn("Enable selinux support"))
	cmd.StringVar(&config.SocketGroup, []string{"G", "-group"}, "docker", usageFn("Group for the unix socket"))
	config.Ulimits = make(map[string]*ulimit.Ulimit)
	cmd.Var(opts.NewUlimitOpt(&config.Ulimits), []string{"-default-ulimit"}, usageFn("Set default ulimits for containers"))
	cmd.BoolVar(&config.Bridge.EnableIPTables, []string{"#iptables", "-iptables"}, true, usageFn("Enable addition of iptables rules"))
	cmd.BoolVar(&config.Bridge.EnableIPForward, []string{"#ip-forward", "-ip-forward"}, true, usageFn("Enable net.ipv4.ip_forward"))
	cmd.BoolVar(&config.Bridge.EnableIPMasq, []string{"-ip-masq"}, true, usageFn("Enable IP masquerading"))
	cmd.BoolVar(&config.Bridge.EnableIPv6, []string{"-ipv6"}, false, usageFn("Enable IPv6 networking"))
	cmd.StringVar(&config.Bridge.IP, []string{"#bip", "-bip"}, "", usageFn("Specify network bridge IP"))
	cmd.StringVar(&config.Bridge.Iface, []string{"b", "-bridge"}, "", usageFn("Attach containers to a network bridge"))
	cmd.StringVar(&config.Bridge.FixedCIDR, []string{"-fixed-cidr"}, "", usageFn("IPv4 subnet for fixed IPs"))
	cmd.StringVar(&config.Bridge.FixedCIDRv6, []string{"-fixed-cidr-v6"}, "", usageFn("IPv6 subnet for fixed IPs"))
	cmd.Var(opts.NewIPOpt(&config.Bridge.DefaultGatewayIPv4, ""), []string{"-default-gateway"}, usageFn("Container default gateway IPv4 address"))
	cmd.Var(opts.NewIPOpt(&config.Bridge.DefaultGatewayIPv6, ""), []string{"-default-gateway-v6"}, usageFn("Container default gateway IPv6 address"))
	cmd.BoolVar(&config.Bridge.InterContainerCommunication, []string{"#icc", "-icc"}, true, usageFn("Enable inter-container communication"))
	cmd.Var(opts.NewIPOpt(&config.Bridge.DefaultIP, "0.0.0.0"), []string{"#ip", "-ip"}, usageFn("Default IP when binding container ports"))
	cmd.BoolVar(&config.Bridge.EnableUserlandProxy, []string{"-userland-proxy"}, false, usageFn("Use userland proxy for loopback traffic"))
	cmd.BoolVar(&config.EnableCors, []string{"#api-enable-cors", "#-api-enable-cors"}, false, usageFn("Enable CORS headers in the remote API, this is deprecated by --api-cors-header"))
	cmd.StringVar(&config.CorsHeaders, []string{"-api-cors-header"}, "", usageFn("Set CORS headers in the remote API"))
	flag.StringVar(&config.RemappedRoot, []string{"-root"}, "", "User/Group [user|uid[:gid|:group]] for container root")

	config.attachExperimentalFlags(cmd, usageFn)
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
		luser, err := user.LookupUser(idparts[0])
		if err != nil {
			return 0, 0, fmt.Errorf("Error during uid lookup for %q: %v", idparts[0], err)
		}
		userId = luser.Uid
		if len(idparts) == 1 {
			// we only have a string username, and no group specified; look up gid from username as group
			group, err := user.LookupGroup(idparts[0])
			if err != nil {
				return 0, 0, fmt.Errorf("Error during gid lookup for %q: %v", idparts[0], err)
			}
			groupId = group.Gid
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
			group, err := user.LookupGroup(idparts[1])
			if err != nil {
				return 0, 0, fmt.Errorf("Error during gid lookup for %q: %v", idparts[1], err)
			}
			groupId = group.Gid
		}
	}
	return userId, groupId, nil
}
