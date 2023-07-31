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

package compute

const (
	GUEST_RESCUE_BASE_PATH = "/opt/cloud/workspace/rescue"
	GUEST_RESCUE_INITRAMFS = "initramfs"
	GUEST_RESCUE_KERNEL    = "kernel"
	GUEST_RESCUE_ROOT      = "root.qcow2"
)

type SGuestRescueConfig struct {
	// Guest OS store path
	BasePath string `json:"base_path"`
	// Initramfs name
	Initd string `json:"initd"`
	// Kernel name
	Kernel string `json:"kernel"`
	// Root image name
	Root string `json:"root"`
	// damaged data disk
	RescueDiskID string `json:"rescue_id"`
}
