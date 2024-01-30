/*
 * config.go - Parsing for our global config file. The file is simply the JSON
 * output of the Config protocol buffer.
 *
 * Copyright 2017 Google Inc.
 * Author: Joe Richey (joerichey@google.com)
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not
 * use this file except in compliance with the License. You may obtain a copy of
 * the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
 * License for the specific language governing permissions and limitations under
 * the License.
 */

// Package metadata contains all of the on disk structures.
// These structures are defined in metadata.proto. The package also
// contains functions for manipulating these structures, specifically:
//    * Reading and Writing the Config file to disk
//    * Getting and Setting Policies for directories
//    * Reasonable defaults for a Policy's EncryptionOptions
package metadata

import (
	"io"

	"google.golang.org/protobuf/encoding/protojson"
)

// WriteConfig outputs the Config data as nicely formatted JSON
func WriteConfig(config *Config, out io.Writer) error {
	m := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "\t",
		UseProtoNames:   true,
		UseEnumNumbers:  false,
		EmitUnpopulated: true,
	}
	bytes, err := m.Marshal(config)
	if err != nil {
		return err
	}
	if _, err = out.Write(bytes); err != nil {
		return err
	}
	_, err = out.Write([]byte{'\n'})
	return err
}

// ReadConfig writes the JSON data into the config structure
func ReadConfig(in io.Reader) (*Config, error) {
	bytes, err := io.ReadAll(in)
	if err != nil {
		return nil, err
	}
	config := new(Config)
	// Discard unknown fields for forwards compatibility.
	u := protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}
	return config, u.Unmarshal(bytes, config)
}
