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

	api "yunion.io/x/onecloud/pkg/apis/compute"
	"yunion.io/x/onecloud/pkg/cloudcommon/db"
	"yunion.io/x/onecloud/pkg/cloudcommon/db/taskman"
	"yunion.io/x/onecloud/pkg/compute/models"
	"yunion.io/x/onecloud/pkg/util/logclient"
)

type StartGuestRescueTask struct {
	SGuestBaseTask
}

func init() {
	taskman.RegisterTask(StartGuestRescueTask{})
}

func (self *StartGuestRescueTask) OnInit(ctx context.Context, obj db.IStandaloneModel, data jsonutils.JSONObject) {
	// Flow: stop -> modify startvm script for rescue -> start
	guest := obj.(*models.SGuest)
	// Check if guest is running
	if guest.Status == api.VM_RUNNING {
		self.StopServer(ctx, guest)
	} else {
		self.PrepareRescue(ctx, guest)
	}
}

func (self *StartGuestRescueTask) StopServer(ctx context.Context, guest *models.SGuest) {
	db.OpsLog.LogEvent(guest, db.ACT_STOPPING, nil, self.UserCred)
	guest.SetStatus(self.UserCred, api.VM_STOPPING, "")
	self.SetStage("OnServerStopComplete", nil)
	guest.StartGuestStopTask(ctx, self.UserCred, true, false, self.GetTaskId())
}

func (self *StartGuestRescueTask) OnServerStopComplete(ctx context.Context, guest *models.SGuest, data jsonutils.JSONObject) {
	db.OpsLog.LogEvent(guest, db.ACT_STOP, guest.GetShortDesc(ctx), self.UserCred)
	logclient.AddActionLogWithStartable(self, guest, logclient.ACT_VM_STOP, guest.GetShortDesc(ctx), self.UserCred, true)

	self.PrepareRescue(ctx, guest)
}

func (self *StartGuestRescueTask) OnServerStopCompleteFailed(ctx context.Context, guest *models.SGuest, err jsonutils.JSONObject) {
	guest.SetStatus(self.UserCred, api.VM_STOP_FAILED, err.String())
	db.OpsLog.LogEvent(guest, db.ACT_STOP_FAIL, err, self.UserCred)
	logclient.AddActionLogWithStartable(self, guest, logclient.ACT_VM_STOP, err, self.UserCred, false)
	self.SetStageFailed(ctx, err)
}

func (self *StartGuestRescueTask) PrepareRescue(ctx context.Context, guest *models.SGuest) {
	db.OpsLog.LogEvent(guest, db.ACT_RESCUING, nil, self.UserCred)
	guest.SetStatus(self.UserCred, api.VM_START_RESCUE, "")
	self.SetStage("OnRescuePrepareComplete", nil)

	host, _ := guest.GetHost()
	err := guest.GetDriver().RequestGuestRescue(ctx, self, nil, host, guest)
	if err != nil {
		self.OnRescuePrepareCompleteFailed(ctx, guest, jsonutils.NewString(err.Error()))
		return
	}
}

func (self *StartGuestRescueTask) OnRescuePrepareComplete(ctx context.Context, guest *models.SGuest, data jsonutils.JSONObject) {
	db.OpsLog.LogEvent(guest, db.ACT_RESCUING, guest.GetShortDesc(ctx), self.UserCred)
	logclient.AddActionLogWithStartable(self, guest, logclient.ACT_VM_RESCUE, guest.GetShortDesc(ctx), self.UserCred, true)
	guest.UpdateRescueMode(true)
	self.RescueStartServer(ctx, guest)
}

func (self *StartGuestRescueTask) OnRescuePrepareCompleteFailed(ctx context.Context, guest *models.SGuest, err jsonutils.JSONObject) {
	guest.SetStatus(self.UserCred, api.VM_RESCUE_FAILED, err.String())
	db.OpsLog.LogEvent(guest, db.ACT_STOP_FAIL, err, self.UserCred)
	logclient.AddActionLogWithStartable(self, guest, logclient.ACT_VM_STOP, err, self.UserCred, false)
	guest.UpdateRescueMode(false)
	self.SetStageFailed(ctx, err)
}

func (self *StartGuestRescueTask) RescueStartServer(ctx context.Context, guest *models.SGuest) {
	guest.SetStatus(self.UserCred, api.VM_START_RESCUE, "")
	self.SetStage("OnRescueStartServerComplete", nil)

	// Set Guest rescue params to guest start params
	host, _ := guest.GetHost()
	err := guest.GetDriver().RequestStartOnHost(ctx, guest, host, self.UserCred, self)
	if err != nil {
		self.OnRescueStartServerCompleteFailed(ctx, guest, jsonutils.NewString(err.Error()))
		return
	}
}

func (self *StartGuestRescueTask) OnRescueStartServerComplete(ctx context.Context, guest *models.SGuest, data jsonutils.JSONObject) {
	db.OpsLog.LogEvent(guest, db.ACT_RESCUE, guest.GetShortDesc(ctx), self.UserCred)
	logclient.AddActionLogWithStartable(self, guest, logclient.ACT_VM_RESCUE, guest.GetShortDesc(ctx), self.UserCred, true)

	// Set guest status to rescue running
	guest.SetStatus(self.UserCred, api.VM_RESCUE_RUNNING, "")
	self.SetStageComplete(ctx, nil)
}

func (self *StartGuestRescueTask) OnRescueStartServerCompleteFailed(ctx context.Context, obj db.IStandaloneModel, err jsonutils.JSONObject) {
	guest := obj.(*models.SGuest)
	guest.SetStatus(self.UserCred, api.VM_RESCUE_FAILED, err.String())
	db.OpsLog.LogEvent(guest, db.ACT_RESCUE_FAIL, guest.GetShortDesc(ctx), self.UserCred)
	logclient.AddActionLogWithStartable(self, guest, logclient.ACT_VM_START, guest.GetShortDesc(ctx), self.UserCred, true)
}

type StopGuestRescueTask struct {
	SGuestBaseTask
}

func init() {
	taskman.RegisterTask(StopGuestRescueTask{})
}

func (self *StopGuestRescueTask) OnInit(ctx context.Context, obj db.IStandaloneModel, data jsonutils.JSONObject) {
	// Flow: stop -> modify startvm script for rescue -> start
	guest := obj.(*models.SGuest)
	// Check if guest is running
	if guest.Status == api.VM_RUNNING {
		self.StopServer(ctx, guest)
	} else {
		self.ClearRescue(ctx, guest)
	}
}

func (self *StopGuestRescueTask) StopServer(ctx context.Context, guest *models.SGuest) {
	db.OpsLog.LogEvent(guest, db.ACT_STOPPING, nil, self.UserCred)
	guest.SetStatus(self.UserCred, api.VM_STOPPING, "")
	self.SetStage("OnServerStopComplete", nil)
	guest.StartGuestStopTask(ctx, self.UserCred, true, false, self.GetTaskId())
}

func (self *StopGuestRescueTask) OnServerStopComplete(ctx context.Context, guest *models.SGuest, data jsonutils.JSONObject) {
	db.OpsLog.LogEvent(guest, db.ACT_STOP, guest.GetShortDesc(ctx), self.UserCred)
	logclient.AddActionLogWithStartable(self, guest, logclient.ACT_VM_STOP, guest.GetShortDesc(ctx), self.UserCred, true)

	self.ClearRescue(ctx, guest)
}

func (self *StopGuestRescueTask) OnServerStopCompleteFailed(ctx context.Context, guest *models.SGuest, err jsonutils.JSONObject) {
	guest.SetStatus(self.UserCred, api.VM_STOP_FAILED, err.String())
	db.OpsLog.LogEvent(guest, db.ACT_STOP_FAIL, err, self.UserCred)
	logclient.AddActionLogWithStartable(self, guest, logclient.ACT_VM_STOP, err, self.UserCred, false)
	self.SetStageFailed(ctx, err)
}

func (self *StopGuestRescueTask) ClearRescue(ctx context.Context, guest *models.SGuest) {
	db.OpsLog.LogEvent(guest, db.ACT_RESCUING, nil, self.UserCred)
	guest.SetStatus(self.UserCred, api.VM_STOP_RESCUE, "")
	self.SetStage("OnRescueClearComplete", nil)

	host, _ := guest.GetHost()
	err := guest.GetDriver().RequestGuestRescueStop(ctx, self, nil, host, guest)
	if err != nil {
		self.OnRescueClearCompleteFailed(ctx, guest, jsonutils.NewString(err.Error()))
		return
	}
}

func (self *StopGuestRescueTask) OnRescueClearComplete(ctx context.Context, guest *models.SGuest, data jsonutils.JSONObject) {
	db.OpsLog.LogEvent(guest, db.ACT_RESCUE_STOP, guest.GetShortDesc(ctx), self.UserCred)
	logclient.AddActionLogWithStartable(self, guest, logclient.ACT_VM_RESCUE_STOP, guest.GetShortDesc(ctx), self.UserCred, true)
	guest.UpdateRescueMode(true)
	self.RescueStartServer(ctx, guest)
}

func (self *StopGuestRescueTask) OnRescueClearCompleteFailed(ctx context.Context, guest *models.SGuest, err jsonutils.JSONObject) {
	guest.SetStatus(self.UserCred, api.VM_RESCUE_FAILED, err.String())
	db.OpsLog.LogEvent(guest, db.ACT_RESCUE_STOP_FAIL, err, self.UserCred)
	logclient.AddActionLogWithStartable(self, guest, logclient.ACT_VM_RESCUE_STOP, err, self.UserCred, false)
	guest.UpdateRescueMode(false)
	self.SetStageFailed(ctx, err)
}

func (self *StopGuestRescueTask) RescueStartServer(ctx context.Context, guest *models.SGuest) {
	guest.SetStatus(self.UserCred, api.VM_STARTING, "")
	self.SetStage("OnRescueStartServerComplete", nil)

	// Set Guest rescue params to guest start params
	host, _ := guest.GetHost()
	err := guest.GetDriver().RequestStartOnHost(ctx, guest, host, self.UserCred, self)
	if err != nil {
		self.OnRescueStartServerCompleteFailed(ctx, guest, jsonutils.NewString(err.Error()))
		return
	}
}

func (self *StopGuestRescueTask) OnRescueStartServerComplete(ctx context.Context, guest *models.SGuest, data jsonutils.JSONObject) {
	db.OpsLog.LogEvent(guest, db.ACT_START, guest.GetShortDesc(ctx), self.UserCred)
	logclient.AddActionLogWithStartable(self, guest, logclient.ACT_VM_RESCUE, guest.GetShortDesc(ctx), self.UserCred, true)

	// Set guest status to rescue running
	guest.SetStatus(self.UserCred, api.VM_STARTING, "")
	self.SetStageComplete(ctx, nil)
}

func (self *StopGuestRescueTask) OnRescueStartServerCompleteFailed(ctx context.Context, obj db.IStandaloneModel, err jsonutils.JSONObject) {
	guest := obj.(*models.SGuest)
	guest.SetStatus(self.UserCred, api.VM_START_FAILED, err.String())
	db.OpsLog.LogEvent(guest, db.ACT_START_FAIL, guest.GetShortDesc(ctx), self.UserCred)
	logclient.AddActionLogWithStartable(self, guest, logclient.ACT_VM_START, guest.GetShortDesc(ctx), self.UserCred, true)
}
