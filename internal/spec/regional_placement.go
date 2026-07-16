// SPDX-License-Identifier: Apache-2.0
// Copyright 2024 Cloudbase Solutions SRL
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//    License for the specific language governing permissions and limitations
//    under the License.

package spec

import (
	"fmt"
	"strings"
)

type RegionalPlacement struct {
	Zones []string `json:"zones" jsonschema:"description=Compute Engine zones allowed for regional placement. All zones must belong to one region."`
}

func (p *RegionalPlacement) Validate() error {
	if p == nil {
		return nil
	}
	if len(p.Zones) == 0 {
		return fmt.Errorf("regional_placement.zones must not be empty")
	}

	region := ""
	seen := make(map[string]struct{}, len(p.Zones))
	for _, zone := range p.Zones {
		zoneRegion, err := regionFromZone(zone)
		if err != nil {
			return fmt.Errorf("invalid regional placement zone '%s': %w", zone, err)
		}
		if region == "" {
			region = zoneRegion
		} else if region != zoneRegion {
			return fmt.Errorf("regional placement zones must belong to one region")
		}
		if _, ok := seen[zone]; ok {
			return fmt.Errorf("duplicate regional placement zone '%s'", zone)
		}
		seen[zone] = struct{}{}
	}
	return nil
}

func (p *RegionalPlacement) Region() string {
	if p == nil || len(p.Zones) == 0 {
		return ""
	}
	region, _ := regionFromZone(p.Zones[0])
	return region
}

func regionFromZone(zone string) (string, error) {
	lastDash := strings.LastIndex(zone, "-")
	suffix := zone[lastDash+1:]
	if lastDash <= 0 || len(suffix) != 1 || suffix[0] < 'a' || suffix[0] > 'z' {
		return "", fmt.Errorf("expected a zone name with a hyphen-delimited suffix")
	}
	return zone[:lastDash], nil
}
