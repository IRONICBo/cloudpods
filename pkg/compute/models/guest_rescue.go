// Copyright 2019 Yunion
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package models

import (
	"context"
	"fmt"

	"yunion.io/x/jsonutils"
	"yunion.io/x/pkg/errors"
	"yunion.io/x/pkg/util/qemuimgfmt"

	api "yunion.io/x/onecloud/pkg/apis/compute"
	"yunion.io/x/onecloud/pkg/cloudcommon/db"
	"yunion.io/x/onecloud/pkg/cloudcommon/db/lockman"
	"yunion.io/x/onecloud/pkg/cloudcommon/db/quotas"
	"yunion.io/x/onecloud/pkg/cloudcommon/db/taskman"
	"yunion.io/x/onecloud/pkg/compute/options"
	"yunion.io/x/onecloud/pkg/mcclient"
	"yunion.io/x/onecloud/pkg/util/logclient"
)

func (disk *SDisk) PerformRescue(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject,
	data jsonutils.JSONObject) (jsonutils.JSONObject, error) {
	// TODO: Check user

	// Get rescue disk path
	params := data.(*jsonutils.JSONDict)
	diskInfo := DiskManager.FetchDiskById(disk.Id)
	if diskInfo != nil {
		params.Add(jsonutils.NewString(diskInfo.AccessPath), "disk_access_path")
	}

	// Get disk host info
	host, err := getHostByDiskId(disk.Id)
	if err != nil {
		return nil, err
	}

	// Create rescue vm
	name := fmt.Sprintf("AUTO_GENERATE_RESCUE_VM-%s", diskInfo.Name)
	guest, err := GuestManager.newRescueVM(ctx, userCred, host, name)
	if err != nil {
		return nil, err
	}

	// Create rescue system disk
	_, err = DiskManager.newRescueSysDisk(ctx, userCred, guest, diskInfo.StorageId)
	if err != nil {
		return nil, err
	}

	// Start rescue vm task
	err = guest.StartGuestRescueTask(ctx, userCred, params, "")

	// Now it only support kvm guest os rescue
	return nil, err
}

// getHostByDiskId Get host info by disk id
func getHostByDiskId(diskId string) (*SHost, error) {
	diskInfo := DiskManager.FetchDiskById(diskId)
	if diskInfo == nil {
		return nil, errors.Errorf("getHostByDiskId: FetchDiskById fail %s", diskId)
	}

	storageInfo := StorageManager.FetchStorageById(diskInfo.StorageId)
	// Now it only support local storage
	if storageInfo == nil && storageInfo.Source != "local" {
		return nil, errors.Errorf("getHostByDiskId: FetchStorageById fail %s", diskInfo.StorageId)
	}

	hosts, err := storageInfo.GetAttachedHosts()
	if err != nil {
		return nil, err
	}

	return &hosts[0], nil
}

// newRescueDisk Create rescue system disk
func (manager *SDiskManager) newRescueSysDisk(ctx context.Context, userCred mcclient.TokenCredential, guest *SGuest, storageId string) (*SDisk, error) {
	disksConfs := make([]*api.DiskConfig, 0)
	diskConf := &api.DiskConfig{
		//Index:    -1, // do not mount
		Index:    0, // do not mount
		SizeMb:   500,
		DiskType: api.DISK_TYPE_SYS,
		Format:   qemuimgfmt.QCOW2.String(),
		Driver:   api.DISK_DRIVER_VIRTIO,
		Backend:  api.STORAGE_LOCAL, // create rescue disk on local storage
		Medium:   api.DISK_TYPE_ROTATE,
		Storage:  storageId,
	}

	// Get host info
	host, _ := guest.GetHost()
	if host == nil {
		logclient.AddActionLogWithContext(ctx, guest, logclient.ACT_CREATE, "No valid host", userCred, false)
		return nil, errors.Errorf("No valid host")
	}

	// Judge available storage
	storage := host.GetLeastUsedStorage(diskConf.Backend)
	if storage == nil {
		logclient.AddActionLogWithContext(ctx, guest, logclient.ACT_CREATE, "No valid storage on current host", userCred, false)
		return nil, errors.Errorf("No valid storage on current host")
	}
	if storage.GetCapacity() > 0 && storage.GetCapacity() < int64(diskConf.SizeMb) {
		logclient.AddActionLogWithContext(ctx, guest, logclient.ACT_CREATE, "Not eough storage space on current host", userCred, false)
		return nil, errors.Errorf("Not eough storage space on current host")
	}

	// Check quota
	pendingUsage := &SQuota{
		Storage: diskConf.SizeMb,
	}
	keys, err := guest.GetQuotaKeys()
	if err != nil {
		return nil, err
	}
	pendingUsage.SetKeys(keys)
	err = quotas.CheckSetPendingQuota(ctx, userCred, pendingUsage)
	if err != nil {
		logclient.AddActionLogWithContext(ctx, guest, logclient.ACT_CREATE, err.Error(), userCred, false)
		return nil, err
	}

	disksConfs = append(disksConfs, diskConf)

	// Start create disk task
	lockman.LockObject(ctx, host)
	defer lockman.ReleaseObject(ctx, host)
	err = guest.CreateDisksOnHost(ctx, userCred, host, disksConfs, pendingUsage, false, false, nil, nil, false)
	if err != nil {
		quotas.CancelPendingUsage(ctx, userCred, pendingUsage, pendingUsage, false)
		logclient.AddActionLogWithContext(ctx, guest, logclient.ACT_CREATE, err.Error(), userCred, false)
		return nil, err
	}

	// Start create disk task
	err = guest.StartGuestCreateDiskTask(ctx, userCred, disksConfs, "")

	// TODO: return a disk info???
	return nil, err
}

// newRescueVM Create rescue vm
func (manager *SGuestManager) newRescueVM(ctx context.Context, userCred mcclient.TokenCredential, host *SHost, name string) (*SGuest, error) {
	// TODO: Check user

	// Init a new rescue vm config
	guest := SGuest{}
	guest.SetModelManager(manager, &guest)

	guest.Hypervisor = api.HYPERVISOR_KVM
	guest.HostId = host.Id
	guest.VcpuCount = 1
	guest.VmemSize = 1024 // MB

	var err = func() error {
		lockman.LockRawObject(ctx, manager.Keyword(), "name")
		defer lockman.ReleaseRawObject(ctx, manager.Keyword(), "name")

		if options.Options.EnableSyncName {
			guest.Name = name
		} else {
			newName, err := db.GenerateName(ctx, manager, userCred, name)
			if err != nil {
				return errors.Wrapf(err, "db.GenerateName")
			}
			guest.Name = newName
		}
		guest.Hostname = guest.Name

		return manager.TableSpec().InsertOrUpdate(ctx, &guest)
	}()
	if err != nil {
		return nil, errors.Wrapf(err, "Insert")
	}

	db.OpsLog.LogEvent(&guest, db.ACT_CREATE, guest.GetShortDesc(ctx), userCred)

	return &guest, nil
}

func (self *SGuest) StartGuestRescueTask(ctx context.Context, userCred mcclient.TokenCredential, data *jsonutils.JSONDict, parentTaskId string) error {
	// Now only support KVM
	_ = self.SetStatus(userCred, api.VM_START_START, "")
	taskName := "GuestRescueTask"
	task, err := taskman.TaskManager.NewTask(ctx, taskName, self, userCred, data, parentTaskId, "", nil)
	if err != nil {
		return err
	}
	err = task.ScheduleRun(nil)
	if err != nil {
		return err
	}
	return nil
}
