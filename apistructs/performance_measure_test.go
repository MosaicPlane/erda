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

package apistructs

import (
	"encoding/json"
	"testing"
)

func TestUserIdentifierUnmarshalJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want UserIdentifier
	}{
		{name: "OIDC subject mapping ID", raw: `"07db26cd-2c1f-4ef0-ace5-8aa3270c831c"`, want: "07db26cd-2c1f-4ef0-ace5-8aa3270c831c"},
		{name: "legacy numeric ID", raw: `12345`, want: "12345"},
		{name: "empty", raw: `null`, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got UserIdentifier
			if err := json.Unmarshal([]byte(tc.raw), &got); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUserIdentifierRejectsInvalidUnquotedValue(t *testing.T) {
	var id UserIdentifier
	if err := json.Unmarshal([]byte(`not-an-id`), &id); err == nil {
		t.Fatal("expected invalid unquoted identifier to fail")
	}
}
