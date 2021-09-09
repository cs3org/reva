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

package alerting

import (
	"fmt"

	"github.com/cs3org/reva/pkg/siteacc/config"
	"github.com/cs3org/reva/pkg/smtpclient"
	"github.com/pkg/errors"
	"github.com/prometheus/alertmanager/template"
	"github.com/rs/zerolog"
)

// Dispatcher is used to dispatch Prometheus alerts via email.
type Dispatcher struct {
	conf *config.Configuration
	log  *zerolog.Logger

	smtp *smtpclient.SMTPCredentials
}

func (dispatcher *Dispatcher) initialize(conf *config.Configuration, log *zerolog.Logger) error {
	if conf == nil {
		return errors.Errorf("no configuration provided")
	}
	dispatcher.conf = conf

	if log == nil {
		return errors.Errorf("no logger provided")
	}
	dispatcher.log = log

	// Create the SMTP client
	if conf.Email.SMTP != nil {
		dispatcher.smtp = smtpclient.NewSMTPCredentials(conf.Email.SMTP)
	}

	return nil
}

/*
{
  "version": "4",
  "groupKey": "xxx",
  "truncatedAlerts": 0,
  "status": "firing",
  "receiver": "you",
  "groupLabels": {},
  "commonLabels": {},
  "commonAnnotations": {},
  "externalURL": "<string>",
  "alerts": [
    {
      "status": "<resolved|firing>",
      "labels": {},
      "annotations": {},
      "startsAt": "2002-10-02T15:00:00Z",
      "endsAt": "2002-10-02T15:00:00Z",
      "generatorURL": "<string>",
      "fingerprint": "<string>"
    }
  ]
}
*/

// DispatchAlerts sends the provided alert(s) via email to the appropriate recipients.
func (dispatcher *Dispatcher) DispatchAlerts(alerts *template.Data) error {
	fmt.Println(alerts)
	return nil
}

// NewDispatcher creates a new dispatcher instance.
func NewDispatcher(conf *config.Configuration, log *zerolog.Logger) (*Dispatcher, error) {
	dispatcher := &Dispatcher{}
	if err := dispatcher.initialize(conf, log); err != nil {
		return nil, errors.Wrapf(err, "unable to initialize the alerts dispatcher")
	}
	return dispatcher, nil
}
