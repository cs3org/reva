// Copyright 2018-2020 CERN
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
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package eos

import (
	"bytes"
	"encoding/gob"

	"github.com/cs3org/reva/pkg/storage"
	"github.com/cs3org/reva/pkg/storage/fs/registry"
	"github.com/cs3org/reva/pkg/storage/utils/eosfs"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

func init() {
	registry.Register("eos", New)
}

type config struct {
	// Namespace for metadata operations
	Namespace string `mapstructure:"namespace" docs:"/"`

	// ShadowNamespace for storing shadow data
	ShadowNamespace string `mapstructure:"shadow_namespace" docs:"/.shadow"`

	// UploadsNamespace for storing upload data
	UploadsNamespace string `mapstructure:"uploads_namespace" docs:"/.uploads"`

	// ShareFolder defines the name of the folder in the
	// shadowed namespace. Ex: /eos/user/.shadow/h/hugo/MyShares
	ShareFolder string `mapstructure:"share_folder" docs:"/MyShares"`

	// Location of the eos binary.
	// Default is /usr/bin/eos.
	EosBinary string `mapstructure:"eos_binary" docs:"/usr/bin/eos"`

	// Location of the xrdcopy binary.
	// Default is /usr/bin/xrdcopy.
	XrdcopyBinary string `mapstructure:"xrdcopy_binary" docs:"/usr/bin/xrdcopy"`

	// URL of the Master EOS MGM.
	// Default is root://eos-example.org
	MasterURL string `mapstructure:"master_url" docs:"root://eos-example.org"`

	// URL of the Slave EOS MGM.
	// Default is root://eos-example.org
	SlaveURL string `mapstructure:"slave_url" docs:"root://eos-example.org"`

	// Location on the local fs where to store reads.
	// Defaults to os.TempDir()
	CacheDirectory string `mapstructure:"cache_directory" docs:"/var/tmp/"`

	// SecProtocol specifies the xrootd security protocol to use between the server and EOS.
	SecProtocol string `mapstructure:"sec_protocol" docs:"-"`

	// Keytab specifies the location of the keytab to use to authenticate to EOS.
	Keytab string `mapstructure:"keytab" docs:"-"`

	// SingleUsername is the username to use when SingleUserMode is enabled
	SingleUsername string `mapstructure:"single_username" docs:"-"`

	// Enables logging of the commands executed
	// Defaults to false
	EnableLogging bool `mapstructure:"enable_logging" docs:"false"`

	// ShowHiddenSysFiles shows internal EOS files like
	// .sys.v# and .sys.a# files.
	ShowHiddenSysFiles bool `mapstructure:"show_hidden_sys_files" docs:"false"`

	// ForceSingleUserMode will force connections to EOS to use SingleUsername
	ForceSingleUserMode bool `mapstructure:"force_single_user_mode" docs:"false"`

	// UseKeyTabAuth changes will authenticate requests by using an EOS keytab.
	UseKeytab bool `mapstructure:"use_keytab" docs:"false"`

	// GatewaySvc stores the endpoint at which the GRPC gateway is exposed.
	GatewaySvc string `mapstructure:"gatewaysvc"`
}

func parseConfig(m map[string]interface{}) (*config, error) {
	c := &config{}
	if err := mapstructure.Decode(m, c); err != nil {
		err = errors.Wrap(err, "error decoding conf")
		return nil, err
	}
	return c, nil
}

// New returns a new implementation of the storage.FS interface that connects to EOS.
func New(m map[string]interface{}) (storage.FS, error) {
	c, err := parseConfig(m)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	err = gob.NewEncoder(&buf).Encode(&c)
	if err != nil {
		return nil, err
	}
	var conf eosfs.Config
	err = gob.NewDecoder(&buf).Decode(&conf)
	if err != nil {
		return nil, err
	}

	return eosfs.NewEOSFS(&conf)
}
