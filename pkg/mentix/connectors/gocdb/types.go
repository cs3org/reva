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

package gocdb

type Extension struct {
	Key   string `xml:"KEY"`
	Value string `xml:"VALUE"`
}

type Extensions struct {
	Extensions []*Extension `xml:"EXTENSION"`
}

type ServiceType struct {
	Name        string `xml:"SERVICE_TYPE_NAME"`
	Description string `xml:"SERVICE_TYPE_DESC"`
}

type ServiceTypes struct {
	Types []*ServiceType `xml:"SERVICE_TYPE"`
}

type Site struct {
	ShortName    string     `xml:"SHORT_NAME"`
	OfficialName string     `xml:"OFFICIAL_NAME"`
	Description  string     `xml:"SITE_DESCRIPTION"`
	Homepage     string     `xml:"HOME_URL"`
	Email        string     `xml:"CONTACT_EMAIL"`
	Domain       string     `xml:"DOMAIN>DOMAIN_NAME"`
	Extensions   Extensions `xml:"EXTENSIONS"`
}

type Sites struct {
	Sites []*Site `xml:"SITE"`
}

type ServiceEndpoint struct {
	Name        string     `xml:"NAME"`
	URL         string     `xml:"URL"`
	Type        string     `xml:"INTERFACENAME"`
	IsMonitored string     `xml:"ENDPOINT_MONITORED"`
	Extensions  Extensions `xml:"EXTENSIONS"`
}

type ServiceEndpoints struct {
	Endpoints []*ServiceEndpoint `xml:"ENDPOINT"`
}

type Service struct {
	Host        string           `xml:"HOSTNAME"`
	Type        string           `xml:"SERVICE_TYPE"`
	IsMonitored string           `xml:"NODE_MONITORED"`
	URL         string           `xml:"URL"`
	Endpoints   ServiceEndpoints `xml:"ENDPOINTS"`
	Extensions  Extensions       `xml:"EXTENSIONS"`
}

type Services struct {
	Services []*Service `xml:"SERVICE_ENDPOINT"`
}
