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
	"fmt"
	"strconv"
	"strings"
)

// UserIdentifier accepts both legacy numeric user IDs and opaque OIDC IDs.
// It serializes as a string so downstream metrics preserve the identity value.
type UserIdentifier string

func (id *UserIdentifier) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		*id = ""
		return nil
	}
	if strings.HasPrefix(raw, `"`) {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*id = UserIdentifier(value)
		return nil
	}
	if _, err := strconv.ParseUint(raw, 10, 64); err != nil {
		return fmt.Errorf("invalid user identifier %q", raw)
	}
	*id = UserIdentifier(raw)
	return nil
}

type PersonalEfficiencyRequest struct {
	Start          string                  `json:"start"`
	End            string                  `json:"end"`
	OrgID          uint64                  `json:"orgID"`
	UserID         UserIdentifier          `json:"userID"`
	ProjectIDs     []uint64                `json:"projectIDs"`
	Operations     []ReportFilterOperation `json:"operations"`
	LabelQuerys    []ReportLabelOperation  `json:"labelQuerys"` // deliberately use labelQuerys instead of labelQueries
	GroupByProject bool                    `json:"groupByProject"`
}

type PersonalContributionRequest struct {
	Start          string         `json:"start"`
	End            string         `json:"end"`
	OrgID          uint64         `json:"orgID"`
	UserID         UserIdentifier `json:"userID"`
	UserEmail      string         `json:"userEmail"`
	ProjectIDs     []uint64       `json:"projectIDs"`
	GroupByProject bool           `json:"groupByProject"`
	GroupByDay     bool           `json:"groupByDay"`
}

type FuncPointTrendRequest struct {
	Start          string         `json:"start"`
	End            string         `json:"end"`
	OrgID          uint64         `json:"orgID"`
	UserID         UserIdentifier `json:"userID"`
	ProjectIDs     []uint64       `json:"projectIDs"`
	GroupByProject bool           `json:"groupByProject"`
}
