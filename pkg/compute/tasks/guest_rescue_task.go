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

package tasks

import (
	"context"

	"yunion.io/x/jsonutils"
	"yunion.io/x/log"

	api "yunion.io/x/onecloud/pkg/apis/compute"
	"yunion.io/x/onecloud/pkg/cloudcommon/db"
	"yunion.io/x/onecloud/pkg/cloudcommon/db/taskman"
	"yunion.io/x/onecloud/pkg/compute/models"
	"yunion.io/x/onecloud/pkg/util/logclient"
)

type GuestRescueTask struct {
	SGuestBaseTask
}

func init() {
	taskman.RegisterTask(GuestRescueTask{})
}

func (self *GuestRescueTask) OnInit(ctx context.Context, obj db.IStandaloneModel, data jsonutils.JSONObject) {
	// Flow: stop -> modify startvm script for rescue -> start
	guest := obj.(*models.SGuest)
	self.StopServer(ctx, guest)
}

func (self *GuestRescueTask) StopServer(ctx context.Context, guest *models.SGuest) {
	db.OpsLog.LogEvent(guest, db.ACT_STOPPING, nil, self.UserCred)
	guest.SetStatus(self.UserCred, api.VM_STOPPING, "")
	self.SetStage("OnServerStopComplete", nil)
	guest.StartGuestStopTask(ctx, self.UserCred, true, false, self.GetTaskId())
}

func (self *GuestRescueTask) OnServerStopComplete(ctx context.Context, obj db.IStandaloneModel, data jsonutils.JSONObject) {
	guest := obj.(*models.SGuest)
	db.OpsLog.LogEvent(guest, db.ACT_STOP, guest.GetShortDesc(ctx), self.UserCred)
	logclient.AddActionLogWithStartable(self, guest, logclient.ACT_VM_STOP, guest.GetShortDesc(ctx), self.UserCred, true)

	self.RescueStartServer(ctx, guest)
}

func (self *GuestRescueTask) OnServerStopCompleteFailed(ctx context.Context, obj db.IStandaloneModel, err jsonutils.JSONObject) {
	guest := obj.(*models.SGuest)
	guest.SetStatus(self.UserCred, api.VM_STOP_FAILED, err.String())
	db.OpsLog.LogEvent(guest, db.ACT_STOP_FAIL, err, self.UserCred)
	logclient.AddActionLogWithStartable(self, guest, logclient.ACT_VM_STOP, err, self.UserCred, false)
	self.SetStageFailed(ctx, err)
}

func (self *GuestRescueTask) RescueStartServer(ctx context.Context, guest *models.SGuest) {
	guest.SetStatus(self.UserCred, api.VM_START_RESCUE, "")
	self.SetStage("OnRescueStartServerComplete", nil)

	log.Errorf("GuestRescueTask RescueStartServer %#v", self.GetParams())
	// Set Guest rescue params to guest start params
	guest.StartGueststartTask(ctx, self.UserCred, self.GetParams(), self.GetTaskId())
}

func (self *GuestRescueTask) OnRescueStartServerComplete(ctx context.Context, guest *models.SGuest, data jsonutils.JSONObject) {
	db.OpsLog.LogEvent(guest, db.ACT_RESCUE, guest.GetShortDesc(ctx), self.UserCred)
	//db.OpsLog.LogEvent(guest, db.ACT_START, guest.GetShortDesc(ctx), self.UserCred)
	logclient.AddActionLogWithStartable(self, guest, logclient.ACT_VM_RESCUE, guest.GetShortDesc(ctx), self.UserCred, true)
	//logclient.AddActionLogWithStartable(self, guest, logclient.ACT_VM_START, guest.GetShortDesc(ctx), self.UserCred, true)

	// Set guest status to rescue running
	// Set guest status to rescue running
	guest.SetStatus(self.UserCred, api.VM_RESCUE_RUNNING, "")
	self.SetStageComplete(ctx, nil)
}

func (self *GuestRescueTask) OnRescueStartServerCompleteFailed(ctx context.Context, obj db.IStandaloneModel, err jsonutils.JSONObject) {
	guest := obj.(*models.SGuest)
	guest.SetStatus(self.UserCred, api.VM_RESCUE_FAILED, err.String())
	db.OpsLog.LogEvent(guest, db.ACT_RESCUE_FAIL, guest.GetShortDesc(ctx), self.UserCred)
	logclient.AddActionLogWithStartable(self, guest, logclient.ACT_VM_START, guest.GetShortDesc(ctx), self.UserCred, true)
}
