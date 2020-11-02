/*
 * MIT License
 *
 * Copyright (c) 2020 Daniel Mueller
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package exchange

import (
	"fmt"
	"strings"
	"sync"

	"github.com/rs/zerolog"

	"github.com/cs3org/reva/pkg/mentix/config"
	"github.com/cs3org/reva/pkg/mentix/meshdata"
)

// Exchanger is the base interface for importers and exporters.
type Exchanger interface {
	// Activate activates the exchanger.
	Activate(conf *config.Configuration, log *zerolog.Logger) error
	// Start starts the exchanger; only exchangers which perform periodical background tasks should do something here.
	Start() error
	// Stop stops any running background activities of the exchanger.
	Stop()

	// MeshDataSet returns the mesh data.
	MeshData() *meshdata.MeshData

	// GetName returns the display name of the exchanger.
	GetName() string
}

// BaseExchanger implements basic exchanger functionality common to all exchangers.
type BaseExchanger struct {
	Exchanger

	conf *config.Configuration
	log  *zerolog.Logger

	enabledConnectors []string

	meshData *meshdata.MeshData
	locker   sync.RWMutex
}

// Activate activates the exchanger.
func (exchanger *BaseExchanger) Activate(conf *config.Configuration, log *zerolog.Logger) error {
	if conf == nil {
		return fmt.Errorf("no configuration provided")
	}
	exchanger.conf = conf

	if log == nil {
		return fmt.Errorf("no logger provided")
	}
	exchanger.log = log

	return nil
}

// Start starts the exchanger; only exchanger which perform periodical background tasks should do something here.
func (exchanger *BaseExchanger) Start() error {
	return nil
}

// Stop stops any running background activities of the exchanger.
func (exchanger *BaseExchanger) Stop() {
}

// IsConnectorEnabled checks if the given connector is enabled for the exchanger.
func (exchanger *BaseExchanger) IsConnectorEnabled(id string) bool {
	for _, connectorID := range exchanger.enabledConnectors {
		if connectorID == "*" || strings.EqualFold(connectorID, id) {
			return true
		}
	}
	return false
}

// Config returns the configuration object.
func (exchanger *BaseExchanger) Config() *config.Configuration {
	return exchanger.conf
}

// Log returns the logger object.
func (exchanger *BaseExchanger) Log() *zerolog.Logger {
	return exchanger.log
}

// EnabledConnectors returns the list of all enabled connectors for the exchanger.
func (exchanger *BaseExchanger) EnabledConnectors() []string {
	return exchanger.enabledConnectors
}

// SetEnabledConnectors sets the list of all enabled connectors for the exchanger.
func (exchanger *BaseExchanger) SetEnabledConnectors(connectors []string) {
	exchanger.enabledConnectors = connectors
}

// MeshDataSet returns the stored mesh data.
func (exchanger *BaseExchanger) MeshData() *meshdata.MeshData {
	return exchanger.meshData
}

// SetMeshDataSet sets new mesh data.
func (exchanger *BaseExchanger) SetMeshData(meshData *meshdata.MeshData) {
	exchanger.Locker().Lock()
	defer exchanger.Locker().Unlock()

	exchanger.meshData = meshData
}

// Locker returns the locking object.
func (exchanger *BaseExchanger) Locker() *sync.RWMutex {
	return &exchanger.locker
}
