package idtools

import (
	"fmt"
	"math"
	"os"

	"github.com/docker/docker/pkg/system"
)

// IDMap contains a single entry for user namespace range remapping. An array
// of IDMap entries represents the structure that will be provided to the Linux
// kernel for creating a user namespace.
type IDMap struct {
	ContainerID int `json:"container_id"`
	HostID      int `json:"host_id"`
	Size        int `json:"size"`
}

// MkdirAllAs creates a directory (include any along the path) and then modifies
// ownership to the requested uid/gid.  If the directory already exists, this
// function will still change ownership to the requested uid/gid pair.
func MkdirAllAs(path string, mode os.FileMode, ownerUID, ownerGID int) error {
	return mkdirAs(path, mode, ownerUID, ownerGID, true)
}

// MkdirAs creates a directory and then modifies ownership to the requested uid/gid.
// If the directory already exists, this function still changes ownership
func MkdirAs(path string, mode os.FileMode, ownerUID, ownerGID int) error {
	return mkdirAs(path, mode, ownerUID, ownerGID, false)
}

func mkdirAs(path string, mode os.FileMode, ownerUID, ownerGID int, mkAll bool) error {

	if mkAll {
		if err := system.MkdirAll(path, mode); err != nil && !os.IsExist(err) {
			return err
		}
	} else {
		if err := os.Mkdir(path, mode); err != nil && !os.IsExist(err) {
			return err
		}
	}
	// even if it existed, we will chown to change ownership as requested
	if err := os.Chown(path, ownerUID, ownerGID); err != nil {
		return err
	}
	return nil
}

// GetRootUIDGID retrieves the remapped root uid/gid pair from the set of maps.
// If the maps are empty, then the root uid/gid will default to "real" 0/0
func GetRootUIDGID(uidMap, gidMap []IDMap) (int, int, error) {
	var uid, gid int

	if uidMap != nil {
		xUID, err := TranslateIDToHost(0, uidMap)
		if err != nil {
			return -1, -1, err
		}
		uid = xUID
	}
	if gidMap != nil {
		xGID, err := TranslateIDToHost(0, gidMap)
		if err != nil {
			return -1, -1, err
		}
		gid = xGID
	}
	return uid, gid, nil
}

// TranslateIDToContainer takes an id mapping, and uses it to translate a
// host ID to the remapped ID. If no map is provided, then the translation
// assumes a 1-to-1 mapping and returns the passed in id
func TranslateIDToContainer(hostID int, idMap []IDMap) (int, error) {

	if idMap == nil {
		return hostID, nil
	}
	for _, m := range idMap {
		if (hostID >= m.HostID) && (hostID <= (m.HostID + m.Size - 1)) {
			contID := m.ContainerID + (hostID - m.HostID)
			return contID, nil
		}
	}
	return -1, fmt.Errorf("Host ID %d cannot be mapped to a container ID", hostID)
}

// TranslateIDToHost takes an id mapping and a remapped ID, and translates the
// ID to the mapped host ID. If no map is provided, then the translation
// assumes a 1-to-1 mapping and returns the passed in id #
func TranslateIDToHost(contID int, idMap []IDMap) (int, error) {

	if idMap == nil {
		return contID, nil
	}
	for _, m := range idMap {
		if (contID >= m.ContainerID) && (contID <= (m.ContainerID + m.Size - 1)) {
			hostID := m.HostID + (contID - m.ContainerID)
			return hostID, nil
		}
	}
	return -1, fmt.Errorf("Container ID %d cannot be mapped to a host ID", contID)
}

// CreateIDMapsForRoot takes a requested remapped uid/gid for root (0,0), and creates
// the proper mapping set for the rest of the uid/gid range.
func CreateIDMapsForRoot(uid, gid int) ([]IDMap, []IDMap, error) {

	// Go and libcontainer expect int (32-bit signed) for uids/gids to handle
	// cross-platform simplicity, so we have to give up on matching Linux
	// uint32 for uid_t and gid_t values
	if uid < 1 || gid < 1 {
		return nil, nil, fmt.Errorf("Cannot create ID maps: uid, gid out of remap range")
	}
	return createIDMap(uid), createIDMap(gid), nil
}

func createIDMap(id int) []IDMap {
	idMap := []IDMap{}

	// The exact id mapping
	idMap = append(idMap, IDMap{
		ContainerID: 0,
		HostID:      id,
		Size:        1,
	})

	// the id mapping from 1 -> id-1
	idMap = append(idMap, IDMap{
		ContainerID: 1,
		HostID:      1,
		Size:        id - 1,
	})

	// the id mapping from id+1 -> ID_MAX
	idMap = append(idMap, IDMap{
		ContainerID: id + 1,
		HostID:      id + 1,
		Size:        math.MaxInt32 - id,
	})

	return idMap
}
