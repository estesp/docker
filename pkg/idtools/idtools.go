package idtools

import (
	"fmt"
	"math"
	"os"

	"github.com/docker/libcontainer/configs"
)

// Create a directory (include any along the path) and modify ownership to the
// requested uid/gid.  If the directory already exists, still changes ownership
func MkdirAllAs(path string, mode os.FileMode, ownerUid, ownerGid int) error {
	return mkdirAs(path, mode, ownerUid, ownerGid, true)
}

// Create a directory and modify ownership to the requested uid/gid.  If the
// directory already exists, still changes ownership
func MkdirAs(path string, mode os.FileMode, ownerUid, ownerGid int) error {
	return mkdirAs(path, mode, ownerUid, ownerGid, false)
}

func mkdirAs(path string, mode os.FileMode, ownerUid, ownerGid int, mkAll bool) error {

	if mkAll {
		if err := os.MkdirAll(path, mode); err != nil && !os.IsExist(err) {
			return err
		}
	} else {
		if err := os.Mkdir(path, mode); err != nil && !os.IsExist(err) {
			return err
		}
	}
	// even if it existed, we will chown to change ownership as requested
	if err := os.Chown(path, ownerUid, ownerGid); err != nil {
		return err
	}
	return nil
}

// Helper function to retrieve remapped root uid/gid in container
// If the maps are empty, then the root uid/gid will default to "real" 0/0
func GetRootUidGid(uidMap, gidMap []configs.IDMap) (int, int, error) {
	var uid, gid int

	if uidMap != nil {
		xUid, err := TranslateIDToHost(0, uidMap)
		if err != nil {
			return -1, -1, err
		}
		uid = xUid
	}
	if gidMap != nil {
		xGid, err := TranslateIDToHost(0, gidMap)
		if err != nil {
			return -1, -1, err
		}
		gid = xGid
	}
	return uid, gid, nil
}

// Given an id mapping, translate a host ID to the proper container ID
// If no map is provided, then the translation assumes a 1-to-1 mapping
// and returns the passed in id #
func TranslateIDToContainer(hostId int, idMap []configs.IDMap) (int, error) {

	if idMap == nil {
		return hostId, nil
	}
	for _, m := range idMap {
		if (hostId >= m.HostID) && (hostId <= (m.HostID + m.Size - 1)) {
			contId := m.ContainerID + (hostId - m.HostID)
			return contId, nil
		}
	}
	return -1, fmt.Errorf("Host ID %d cannot be mapped to a container ID", hostId)
}

// Given an id mapping, translate a container ID to the proper host ID
// If no map is provided, then the translation assumes a 1-to-1 mapping
// and returns the passed in id #
func TranslateIDToHost(contId int, idMap []configs.IDMap) (int, error) {

	if idMap == nil {
		return contId, nil
	}
	for _, m := range idMap {
		if (contId >= m.ContainerID) && (contId <= (m.ContainerID + m.Size - 1)) {
			hostId := m.HostID + (contId - m.ContainerID)
			return hostId, nil
		}
	}
	return -1, fmt.Errorf("Container ID %d cannot be mapped to a host ID", contId)
}

// Create libcontainer/native execdriver consumable uid/gid mappings for a single
// host uid/gid pair to be treated as root.  This is useful for a simple remap of root
// rather than a more complex mapping of multiple IDs on host -> container
func CreateIDMapsForRoot(uid, gid int) ([]configs.IDMap, []configs.IDMap, error) {

	// Go and libcontainer expect int (32-bit signed) for uids/gids to handle
	// cross-platform simplicity, so we have to give up on matching Linux
	// uint32 for uid_t and gid_t values
	if uid < 1 || gid < 1 {
		return nil, nil, fmt.Errorf("Cannot create ID maps: uid, gid out of remap range")
	}
	return createIDMap(uid), createIDMap(gid), nil
}

func createIDMap(id int) []configs.IDMap {
	idMap := []configs.IDMap{}

	// The exact id mapping
	idMap = append(idMap, configs.IDMap{
		ContainerID: 0,
		HostID:      id,
		Size:        1,
	})

	// the id mapping from 1 -> id-1
	idMap = append(idMap, configs.IDMap{
		ContainerID: 1,
		HostID:      1,
		Size:        id - 1,
	})

	// the id mapping from id+1 -> ID_MAX
	idMap = append(idMap, configs.IDMap{
		ContainerID: id + 1,
		HostID:      id + 1,
		Size:        math.MaxInt32 - id,
	})

	return idMap
}
