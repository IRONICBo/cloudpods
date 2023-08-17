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
	"yunion.io/x/pkg/utils"

	api "yunion.io/x/onecloud/pkg/apis/compute"
	"yunion.io/x/onecloud/pkg/cloudcommon/db"
	"yunion.io/x/onecloud/pkg/cloudcommon/db/taskman"
	"yunion.io/x/onecloud/pkg/httperrors"
	"yunion.io/x/onecloud/pkg/mcclient"
)

func (self *SGuest) PerformRescue(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject,
	data jsonutils.JSONObject) (jsonutils.JSONObject, error) {
	if !utils.IsInStringArray(self.Status, []string{api.VM_READY, api.VM_RUNNING}) {
		return nil, httperrors.NewInvalidStatusError("guest status must be ready or running")
	}

	// Check vmem size, need to be greater than 2G
	if self.VmemSize < 2048 {
		return nil, httperrors.NewInvalidStatusError("vmem size must be greater than 2G")
	}

	// Reset index
	disks, err := self.GetGuestDisks()
	if err != nil || len(disks) < 1 {
		return nil, httperrors.NewInvalidStatusError("guest.GetGuestDisks: %s", err.Error())
	}
	for i := 0; i < len(disks); i++ {
		if disks[i].BootIndex >= 0 {
			// Move to next index, and easy to rollback
			err = disks[i].SetBootIndex(disks[i].BootIndex + 1)
			if err != nil {
				return nil, httperrors.NewInvalidStatusError("guest.SetBootIndex: %s", err.Error())
			}
		}
	}

	// Start rescue vm task
	err = self.StartGuestRescueTask(ctx, userCred, data.(*jsonutils.JSONDict), "")
	if err != nil {
		return nil, httperrors.NewInvalidStatusError("guest.StartGuestRescueTask: %s", err.Error())
	}

	// Now it only support kvm guest os rescue
	return nil, nil
}

func (self *SGuest) PerformRescueStop(ctx context.Context, userCred mcclient.TokenCredential, query jsonutils.JSONObject,
	data jsonutils.JSONObject) (jsonutils.JSONObject, error) {
	if !self.RescueMode {
		return nil, httperrors.NewInvalidStatusError("guest is not in rescue mode")
	}

	// Recover index
	disks, err := self.GetGuestDisks()
	if err != nil || len(disks) < 1 {
		return nil, httperrors.NewInvalidStatusError("guest.GetGuestDisks: %s", err.Error())
	}
	for i := 0; i < len(disks); i++ {
		if disks[i].BootIndex >= 0 {
			// Rollback index
			err = disks[i].SetBootIndex(disks[i].BootIndex - 1)
			if err != nil {
				return nil, httperrors.NewInvalidStatusError("guest.SetBootIndex: %s", err.Error())
			}
		}
	}

	// Start rescue vm task
	err = self.StopGuestRescueTask(ctx, userCred, data.(*jsonutils.JSONDict), "")
	if err != nil {
		return nil, httperrors.NewInvalidStatusError("guest.StopGuestRescueTask: %s", err.Error())
	}

	// Now it only support kvm guest os rescue
	return nil, nil
}

func (self *SGuest) UpdateRescueMode(mode bool) error {
	_, err := db.Update(self, func() error {
		self.RescueMode = mode
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "Update RescueMode")
	}
	return nil
}

func (self *SGuest) StartGuestRescueTask(ctx context.Context, userCred mcclient.TokenCredential, data *jsonutils.JSONDict, parentTaskId string) error {
	// Now only support KVM
	taskName := "StartGuestRescueTask"
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

func (self *SGuest) StopGuestRescueTask(ctx context.Context, userCred mcclient.TokenCredential, data *jsonutils.JSONDict, parentTaskId string) error {
	taskName := "StopGuestRescueTask"
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
