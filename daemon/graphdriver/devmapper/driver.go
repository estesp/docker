// +build linux

package devmapper

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/libcontainer/configs"

	"github.com/docker/docker/daemon/graphdriver"
	"github.com/docker/docker/pkg/devicemapper"
	"github.com/docker/docker/pkg/idtools"
	"github.com/docker/docker/pkg/mount"
	"github.com/docker/docker/pkg/units"
)

func init() {
	graphdriver.Register("devicemapper", Init)
}

// Placeholder interfaces, to be replaced
// at integration.

// End of placeholder interfaces.

type Driver struct {
	*DeviceSet
	home    string
	uidMaps []configs.IDMap
	gidMaps []configs.IDMap
}

var backingFs = "<unknown>"

func Init(home string, options []string, uidMaps, gidMaps []configs.IDMap) (graphdriver.Driver, error) {
	fsMagic, err := graphdriver.GetFSMagic(home)
	if err != nil {
		return nil, err
	}
	if fsName, ok := graphdriver.FsNames[fsMagic]; ok {
		backingFs = fsName
	}

	deviceSet, err := NewDeviceSet(home, true, options, uidMaps, gidMaps)
	if err != nil {
		return nil, err
	}

	if err := mount.MakePrivate(home); err != nil {
		return nil, err
	}

	d := &Driver{
		DeviceSet: deviceSet,
		home:      home,
		uidMaps:   uidMaps,
		gidMaps:   gidMaps,
	}

	return graphdriver.NaiveDiffDriver(d, uidMaps, gidMaps), nil
}

func (d *Driver) String() string {
	return "devicemapper"
}

func (d *Driver) Status() [][2]string {
	s := d.DeviceSet.Status()

	status := [][2]string{
		{"Pool Name", s.PoolName},
		{"Pool Blocksize", fmt.Sprintf("%s", units.HumanSize(float64(s.SectorSize)))},
		{"Backing Filesystem", backingFs},
		{"Data file", s.DataFile},
		{"Metadata file", s.MetadataFile},
		{"Data Space Used", fmt.Sprintf("%s", units.HumanSize(float64(s.Data.Used)))},
		{"Data Space Total", fmt.Sprintf("%s", units.HumanSize(float64(s.Data.Total)))},
		{"Data Space Available", fmt.Sprintf("%s", units.HumanSize(float64(s.Data.Available)))},
		{"Metadata Space Used", fmt.Sprintf("%s", units.HumanSize(float64(s.Metadata.Used)))},
		{"Metadata Space Total", fmt.Sprintf("%s", units.HumanSize(float64(s.Metadata.Total)))},
		{"Metadata Space Available", fmt.Sprintf("%s", units.HumanSize(float64(s.Metadata.Available)))},
		{"Udev Sync Supported", fmt.Sprintf("%v", s.UdevSyncSupported)},
	}
	if len(s.DataLoopback) > 0 {
		status = append(status, [2]string{"Data loop file", s.DataLoopback})
	}
	if len(s.MetadataLoopback) > 0 {
		status = append(status, [2]string{"Metadata loop file", s.MetadataLoopback})
	}
	if vStr, err := devicemapper.GetLibraryVersion(); err == nil {
		status = append(status, [2]string{"Library Version", vStr})
	}
	return status
}

func (d *Driver) Cleanup() error {
	err := d.DeviceSet.Shutdown()

	if err2 := mount.Unmount(d.home); err == nil {
		err = err2
	}

	return err
}

func (d *Driver) Create(id, parent string) error {
	if err := d.DeviceSet.AddDevice(id, parent); err != nil {
		return err
	}

	return nil
}

func (d *Driver) Remove(id string) error {
	if !d.DeviceSet.HasDevice(id) {
		// Consider removing a non-existing device a no-op
		// This is useful to be able to progress on container removal
		// if the underlying device has gone away due to earlier errors
		return nil
	}

	// This assumes the device has been properly Get/Put:ed and thus is unmounted
	if err := d.DeviceSet.DeleteDevice(id); err != nil {
		return err
	}

	mp := path.Join(d.home, "mnt", id)
	if err := os.RemoveAll(mp); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (d *Driver) Get(id, mountLabel string) (string, error) {
	mp := path.Join(d.home, "mnt", id)

	uid, gid, err := idtools.GetRootUidGid(d.uidMaps, d.gidMaps)
	if err != nil {
		return "", err
	}
	// Create the target directories if they don't exist
	if err := idtools.MkdirAllAs(path.Join(d.home, "mnt"), 0755, uid, gid); err != nil && !os.IsExist(err) {
		return "", err
	}
	if err := idtools.MkdirAs(mp, 0755, uid, gid); err != nil && !os.IsExist(err) {
		return "", err
	}

	// Mount the device
	if err := d.DeviceSet.MountDevice(id, mp, mountLabel); err != nil {
		return "", err
	}

	rootFs := path.Join(mp, "rootfs")
	if err := idtools.MkdirAllAs(rootFs, 0755, uid, gid); err != nil && !os.IsExist(err) {
		d.DeviceSet.UnmountDevice(id)
		return "", err
	}

	idFile := path.Join(mp, "id")
	if _, err := os.Stat(idFile); err != nil && os.IsNotExist(err) {
		// Create an "id" file with the container/image id in it to help reconscruct this in case
		// of later problems
		if err := ioutil.WriteFile(idFile, []byte(id), 0600); err != nil {
			d.DeviceSet.UnmountDevice(id)
			return "", err
		}
	}

	return rootFs, nil
}

func (d *Driver) Put(id string) error {
	err := d.DeviceSet.UnmountDevice(id)
	if err != nil {
		log.Errorf("Error unmounting device %s: %s", id, err)
	}
	return err
}

func (d *Driver) Exists(id string) bool {
	return d.DeviceSet.HasDevice(id)
}
