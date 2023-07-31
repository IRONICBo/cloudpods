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
	"yunion.io/x/onecloud/pkg/compute/options"
	"yunion.io/x/onecloud/pkg/mcclient/auth"
	"yunion.io/x/onecloud/pkg/mcclient/modules/vpcagent"
	"yunion.io/x/onecloud/pkg/util/logclient"
)

type GuestRescueTask struct {
	SGuestBaseTask
}

func init() {
	taskman.RegisterTask(GuestRescueTask{})
}

func (self *GuestRescueTask) OnInit(ctx context.Context, obj db.IStandaloneModel, data jsonutils.JSONObject) {
	guest := obj.(*models.SGuest)
	db.OpsLog.LogEvent(guest, db.ACT_STARTING, nil, self.UserCred)
	disk_access_path, _ := self.GetParams().GetString("disk_access_path")
	params := data.(*jsonutils.JSONDict)
	params.Add(jsonutils.NewString(disk_access_path), "disk_access_path")
	self.RequestRescue(ctx, guest, params)
}

func (self *GuestRescueTask) RequestRescue(ctx context.Context, guest *models.SGuest, data jsonutils.JSONObject) {
	_ = self.SetStage("OnStartComplete", nil)
	host, _ := guest.GetHost()
	_ = guest.SetStatus(self.UserCred, api.VM_STARTING, "")
	err := guest.GetDriver().RequestGuestRescue(ctx, self.UserCred, data, host, guest)
	if err != nil {
		self.OnStartCompleteFailed(ctx, guest, jsonutils.NewString(err.Error()))
		return
	}
}

func (task *GuestRescueTask) OnStartComplete(ctx context.Context, obj db.IStandaloneModel, data jsonutils.JSONObject) {
	guest := obj.(*models.SGuest)
	isVpc, err := guest.IsOneCloudVpcNetwork()
	if err != nil {
		log.Errorf("IsOneCloudVpcNetwork fail: %s", err)
	} else if isVpc {
		// force update VPC topo
		err := vpcagent.VpcAgent.DoSync(auth.GetAdminSession(ctx, options.Options.Region))
		if err != nil {
			log.Errorf("vpcagent.VpcAgent.DoSync fail %s", err)
		}
	}
	db.OpsLog.LogEvent(guest, db.ACT_START, guest.GetShortDesc(ctx), task.UserCred)
	logclient.AddActionLogWithStartable(task, guest, logclient.ACT_VM_START, guest.GetShortDesc(ctx), task.UserCred, true)
	task.taskComplete(ctx, guest)
}

func (self *GuestRescueTask) OnStartCompleteFailed(ctx context.Context, obj db.IStandaloneModel, err jsonutils.JSONObject) {
	guest := obj.(*models.SGuest)
	_ = guest.SetStatus(self.UserCred, api.VM_START_FAILED, err.String())
	db.OpsLog.LogEvent(guest, db.ACT_START_FAIL, err, self.UserCred)
	logclient.AddActionLogWithStartable(self, guest, logclient.ACT_VM_START, err, self.UserCred, false)
	self.SetStageFailed(ctx, err)
}

func (self *GuestRescueTask) taskComplete(ctx context.Context, guest *models.SGuest) {
	_ = models.HostManager.ClearSchedDescCache(guest.HostId)
	self.SetStageComplete(ctx, nil)
}
