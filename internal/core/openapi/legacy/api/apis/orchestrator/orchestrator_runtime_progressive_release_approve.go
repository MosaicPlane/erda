// Copyright (c) 2026 MosaicPlane Authors.
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

package orchestrator

import "github.com/erda-project/erda/internal/core/openapi/legacy/api/apis"

var ORCHESTRATOR_RUNTIME_PROGRESSIVE_RELEASE_APPROVE = apis.ApiSpec{
	Path:        "/api/runtimes/<runtimeID>/progressive-releases/actions/approve",
	BackendPath: "/api/runtimes/<runtimeID>/progressive-releases/actions/approve",
	Host:        "orchestrator.marathon.l4lb.thisdcos.directory:8081",
	Scheme:      "http",
	Method:      "POST",
	CheckLogin:  true,
	CheckToken:  true,
	Doc:         `人工确认 Runtime 渐进式发布继续执行`,
}
