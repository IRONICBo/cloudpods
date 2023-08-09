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

	"yunion.io/x/jsonutils"
	"yunion.io/x/pkg/errors"
	"yunion.io/x/pkg/util/qemuimgfmt"

	api "yunion.io/x/onecloud/pkg/apis/compute"
	"yunion.io/x/onecloud/pkg/cloudcommon/db/lockman"
	"yunion.io/x/onecloud/pkg/cloudcommon/db/quotas"
	"yunion.io/x/onecloud/pkg/cloudcommon/db/taskman"
	"yunion.io/x/onecloud/pkg/mcclient"
	"yunion.io/x/onecloud/pkg/util/logclient"
)

func (self *SGuest) PerformRescue(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject,
	data jsonutils.JSONObject) (jsonutils.JSONObject, error) {
	// TODO: Check vm status?

	// Check vmem size, need to be greater than 2G
	if self.VmemSize < 2048 {
		return nil, errors.Errorf("vmem size must be greater than 2G")
	}

	// Create rescue system disk
	_, err := DiskManager.newRescueSysDisk(ctx, userCred, self)
	if err != nil {
		return nil, errors.Wrapf(err, "DiskManager.newRescueSysDisk")
	}

	disks, err := self.GetGuestDisks()
	if err != nil || len(disks) <= 1 {
		return nil, errors.Wrapf(err, "guest.GetGuestDisks")
	}

	// Reset index
	for i := 0; i < len(disks)-1; i++ {
		if disks[i].BootIndex >= 0 {
			// Move to next index, and easy to rollback
			err = disks[i].SetBootIndex(disks[i].BootIndex + 1)
			if err != nil {
				return nil, errors.Wrapf(err, "guest.SetBootIndex")
			}
		}
	}
	disks[len(disks)-1].SetBootIndex(1) // Set boot index to first

	// Start rescue vm task
	err = self.StartGuestRescueTask(ctx, userCred, data.(*jsonutils.JSONDict), "")

	// Now it only support kvm guest os rescue
	return nil, err
}

// newRescueDisk Create rescue system disk and mount to guest
func (manager *SDiskManager) newRescueSysDisk(ctx context.Context, userCred mcclient.TokenCredential, guest *SGuest) (*api.DiskConfig, error) {
	disksConfs := make([]*api.DiskConfig, 0)
	diskConf := &api.DiskConfig{
		//Index:    -1, // do not mount
		Index:    0, // mount to guest
		SizeMb:   500,
		DiskType: api.DISK_TYPE_SYS,
		Format:   qemuimgfmt.QCOW2.String(),
		Driver:   api.DISK_DRIVER_VIRTIO,
		Backend:  api.STORAGE_LOCAL, // create rescue disk on local storage
		Medium:   api.DISK_TYPE_ROTATE,
		//Storage:  storageId,
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
	// CreateDisksOnHost will update disksConfs and put DiskId in them.
	err = guest.CreateDisksOnHost(ctx, userCred, host, disksConfs, pendingUsage, false, false, nil, nil, false)
	if err != nil {
		quotas.CancelPendingUsage(ctx, userCred, pendingUsage, pendingUsage, false)
		logclient.AddActionLogWithContext(ctx, guest, logclient.ACT_CREATE, err.Error(), userCred, false)
		return nil, err
	}

	// Start create disk task
	err = guest.StartGuestCreateDiskTask(ctx, userCred, disksConfs, "")

	// diskConf
	return diskConf, err
}

func (self *SGuest) StartGuestRescueTask(ctx context.Context, userCred mcclient.TokenCredential, data *jsonutils.JSONDict, parentTaskId string) error {
	// Now only support KVM
	self.SetStatus(userCred, api.VM_START_RESCUE, "")

	data.Add(jsonutils.JSONTrue, "rescue")
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
