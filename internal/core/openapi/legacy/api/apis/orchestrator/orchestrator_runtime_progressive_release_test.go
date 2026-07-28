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

package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erda-project/erda/internal/core/openapi/legacy/api/apis"
)

func TestProgressiveReleaseOpenAPISpecs(t *testing.T) {
	specs := []struct {
		name   string
		spec   apis.ApiSpec
		path   string
		method string
	}{
		{"get", ORCHESTRATOR_RUNTIME_PROGRESSIVE_RELEASE_GET, "/api/runtimes/<runtimeID>/progressive-releases", "GET"},
		{"update", ORCHESTRATOR_RUNTIME_PROGRESSIVE_RELEASE_UPDATE, "/api/runtimes/<runtimeID>/progressive-releases", "PUT"},
		{"approve", ORCHESTRATOR_RUNTIME_PROGRESSIVE_RELEASE_APPROVE, "/api/runtimes/<runtimeID>/progressive-releases/actions/approve", "POST"},
		{"rollback", ORCHESTRATOR_RUNTIME_PROGRESSIVE_RELEASE_ROLLBACK, "/api/runtimes/<runtimeID>/progressive-releases/actions/rollback", "POST"},
	}

	for _, tt := range specs {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.path, tt.spec.Path)
			require.Equal(t, tt.path, tt.spec.BackendPath)
			require.Equal(t, tt.method, tt.spec.Method)
			require.True(t, tt.spec.CheckLogin)
			require.True(t, tt.spec.CheckToken)
		})
	}
}
