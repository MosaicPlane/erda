// Copyright (c) 2021 Terminus, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package addon

import "testing"

import "github.com/erda-project/erda/internal/tools/orchestrator/dbclient"

func TestAutoInjectMonitorOption(t *testing.T) {
	if !New().autoInjectMonitor {
		t.Fatal("monitor auto-injection must remain enabled by default")
	}
	if New(WithAutoInjectMonitor(false)).autoInjectMonitor {
		t.Fatal("monitor auto-injection option was not applied")
	}
}

func TestShouldIncludeDefaultPrebuild(t *testing.T) {
	monitor := dbclient.AddonPrebuild{AddonName: "monitor"}
	mysql := dbclient.AddonPrebuild{AddonName: "mysql"}

	if !New().shouldIncludeDefaultPrebuild(monitor) {
		t.Fatal("legacy default behavior must retain monitor")
	}
	withoutMonitor := New(WithAutoInjectMonitor(false))
	if withoutMonitor.shouldIncludeDefaultPrebuild(monitor) {
		t.Fatal("disabled monitor injection must filter a stale monitor default")
	}
	if !withoutMonitor.shouldIncludeDefaultPrebuild(mysql) {
		t.Fatal("disabling monitor injection must not filter other addons")
	}
}
