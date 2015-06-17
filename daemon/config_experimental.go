// +build experimental

package daemon

import (
	"fmt"
	"strconv"
	"strings"

	flag "github.com/docker/docker/pkg/mflag"
	"github.com/opencontainers/runc/libcontainer/user"
)

func (config *Config) attachExperimentalFlags(cmd *flag.FlagSet, usageFn func(string) string) {
	cmd.StringVar(&config.DefaultNetwork, []string{"-default-network"}, "", usageFn("Set default network"))
	cmd.StringVar(&config.NetworkKVStore, []string{"-kv-store"}, "", usageFn("Set KV Store configuration"))
	cmd.StringVar(&config.RemappedRoot, []string{"-root"}, "", usageFn("User/Group setting for container root"))
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
