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

package config

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

func NewConfig(cfgFile string) (*Config, error) {
	var config Config
	if _, err := toml.DecodeFile(cfgFile, &config); err != nil {
		return nil, fmt.Errorf("error decoding config: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("error validating config: %w", err)
	}
	return &config, nil
}

type Config struct {
	ProjectId       string `toml:"project_id"`
	Zone            string `toml:"zone"`
	CredentialsFile string `toml:"credentials_file"`
	NetworkID       string `toml:"network_id"`
	SubnetworkID    string `toml:"subnetwork_id"`
}

func (c *Config) Validate() error {
	if c.Zone == "" {
		return fmt.Errorf("missing region")
	}
	if c.ProjectId == "" {
		return fmt.Errorf("missing project_id")
	}
	if c.NetworkID == "" {
		return fmt.Errorf("missing network_id")
	}
	if c.SubnetworkID == "" {
		return fmt.Errorf("missing subnetwork_id")
	}
	if c.CredentialsFile == "" {
		return fmt.Errorf("missing credentials_file")
	}

	return nil
}
