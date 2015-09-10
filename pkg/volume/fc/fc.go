/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package fc

import (
	"strconv"

	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/types"
	"k8s.io/kubernetes/pkg/util"
	"k8s.io/kubernetes/pkg/util/exec"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/volume"
)

// This is the primary entrypoint for volume plugins.
func ProbeVolumePlugins() []volume.VolumePlugin {
	return []volume.VolumePlugin{&fcPlugin{nil, exec.New()}}
}

type fcPlugin struct {
	host volume.VolumeHost
	exe  exec.Interface
}

var _ volume.VolumePlugin = &fcPlugin{}
var _ volume.PersistentVolumePlugin = &fcPlugin{}

const (
	fcPluginName = "kubernetes.io/fc"
)

func (plugin *fcPlugin) Init(host volume.VolumeHost) {
	plugin.host = host
}

func (plugin *fcPlugin) Name() string {
	return fcPluginName
}

func (plugin *fcPlugin) CanSupport(spec *volume.Spec) bool {
	if spec.VolumeSource.FC == nil && spec.PersistentVolumeSource.FC == nil {
		return false
	}
	// TODO:  turn this into a func so CanSupport can be unit tested without
	// having to make system calls
	// see if /sys/class/fc_transport is there, which indicates fc is connected
	_, err := plugin.execCommand("ls", []string{"/sys/class/fc_transport"})
	if err == nil {
		return true
	}

	return false
}

func (plugin *fcPlugin) GetAccessModes() []api.PersistentVolumeAccessMode {
	return []api.PersistentVolumeAccessMode{
		api.ReadWriteOnce,
		api.ReadOnlyMany,
	}
}

func (plugin *fcPlugin) NewBuilder(spec *volume.Spec, pod *api.Pod, _ volume.VolumeOptions, mounter mount.Interface) (volume.Builder, error) {
	// Inject real implementations here, test through the internal function.
	return plugin.newBuilderInternal(spec, pod.UID, &FCUtil{}, mounter)
}

func (plugin *fcPlugin) newBuilderInternal(spec *volume.Spec, podUID types.UID, manager diskManager, mounter mount.Interface) (volume.Builder, error) {
	// fc volumes used directly in a pod have a ReadOnly flag set by the pod author.
	// fc volumes used as a PersistentVolume gets the ReadOnly flag indirectly through the persistent-claim volume used to mount the PV
	var readOnly bool
	var fc *api.FCVolumeSource
	if spec.VolumeSource.FC != nil {
		fc = spec.VolumeSource.FC
		readOnly = fc.ReadOnly
	} else {
		fc = spec.PersistentVolumeSource.FC
		readOnly = spec.ReadOnly
	}

	lun := strconv.Itoa(fc.Lun)

	return &fcDiskBuilder{
		fcDisk: &fcDisk{
			podUID:  podUID,
			volName: spec.Name,
			wwns:    fc.TargetWWNs,
			lun:     lun,
			manager: manager,
			mounter: &mount.SafeFormatAndMount{mounter, exec.New()},
			plugin:  plugin},
		fsType:   fc.FSType,
		readOnly: readOnly,
	}, nil
}

func (plugin *fcPlugin) NewCleaner(volName string, podUID types.UID, mounter mount.Interface) (volume.Cleaner, error) {
	// Inject real implementations here, test through the internal function.
	return plugin.newCleanerInternal(volName, podUID, &FCUtil{}, mounter)
}

func (plugin *fcPlugin) newCleanerInternal(volName string, podUID types.UID, manager diskManager, mounter mount.Interface) (volume.Cleaner, error) {
	return &fcDiskCleaner{&fcDisk{
		podUID:  podUID,
		volName: volName,
		manager: manager,
		mounter: mounter,
		plugin:  plugin,
	}}, nil
}

func (plugin *fcPlugin) execCommand(command string, args []string) ([]byte, error) {
	cmd := plugin.exe.Command(command, args...)
	return cmd.CombinedOutput()
}

type fcDisk struct {
	volName string
	podUID  types.UID
	portal  string
	wwns    []string
	lun     string
	plugin  *fcPlugin
	mounter mount.Interface
	// Utility interface that provides API calls to the provider to attach/detach disks.
	manager diskManager
}

func (fc *fcDisk) GetPath() string {
	name := fcPluginName
	// safe to use PodVolumeDir now: volume teardown occurs before pod is cleaned up
	return fc.plugin.host.GetPodVolumeDir(fc.podUID, util.EscapeQualifiedNameForDisk(name), fc.volName)
}

type fcDiskBuilder struct {
	*fcDisk
	readOnly bool
	fsType   string
}

var _ volume.Builder = &fcDiskBuilder{}

func (b *fcDiskBuilder) SetUp() error {
	return b.SetUpAt(b.GetPath())
}

func (b *fcDiskBuilder) SetUpAt(dir string) error {
	// diskSetUp checks mountpoints and prevent repeated calls
	err := diskSetUp(b.manager, *b, dir, b.mounter)
	if err != nil {
		glog.Errorf("fc: failed to setup")
		return err
	}
	globalPDPath := b.manager.MakeGlobalPDName(*b.fcDisk)
	var options []string
	if b.readOnly {
		options = []string{"remount", "ro"}
	} else {
		options = []string{"remount", "rw"}
	}
	return b.mounter.Mount(globalPDPath, dir, "", options)
}

type fcDiskCleaner struct {
	*fcDisk
}

var _ volume.Cleaner = &fcDiskCleaner{}

func (b *fcDiskBuilder) IsReadOnly() bool {
	return b.readOnly
}

// Unmounts the bind mount, and detaches the disk only if the disk
// resource was the last reference to that disk on the kubelet.
func (c *fcDiskCleaner) TearDown() error {
	return c.TearDownAt(c.GetPath())
}

func (c *fcDiskCleaner) TearDownAt(dir string) error {
	return diskTearDown(c.manager, *c, dir, c.mounter)
}
