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

package guestman

import (
	"context"

	"yunion.io/x/jsonutils"
	"yunion.io/x/log"

	"yunion.io/x/onecloud/pkg/httperrors"
	"yunion.io/x/onecloud/pkg/mcclient"
)

func (m *SGuestManager) GuestRescue(ctx context.Context, userCred mcclient.TokenCredential, sid string, body jsonutils.JSONObject) (jsonutils.JSONObject, error) {
	log.Errorln("GuestRescue body:", body.String())
	res := jsonutils.NewDict()
	diskPath, err := body.GetString("disk_access_path")
	if err != nil {
		return res, httperrors.NewBadRequestError("No disk path provided")
	}
	res.Add(jsonutils.NewString(diskPath), "disk_access_path")

	return res, nil
}
