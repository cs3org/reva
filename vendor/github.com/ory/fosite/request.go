/*
 * Copyright © 2015-2018 Aeneas Rekkas <aeneas+oss@aeneas.io>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * @author		Aeneas Rekkas <aeneas+oss@aeneas.io>
 * @copyright 	2015-2018 Aeneas Rekkas <aeneas+oss@aeneas.io>
 * @license 	Apache-2.0
 *
 */

package fosite

import (
	"net/url"
	"time"

	"github.com/pborman/uuid"
)

// Request is an implementation of Requester
type Request struct {
	ID                string     `json:"id" gorethink:"id"`
	RequestedAt       time.Time  `json:"requestedAt" gorethink:"requestedAt"`
	Client            Client     `json:"client" gorethink:"client"`
	RequestedScope    Arguments  `json:"scopes" gorethink:"scopes"`
	GrantedScope      Arguments  `json:"grantedScopes" gorethink:"grantedScopes"`
	Form              url.Values `json:"form" gorethink:"form"`
	Session           Session    `json:"session" gorethink:"session"`
	RequestedAudience Arguments  `json:"requestedAudience"`
	GrantedAudience   Arguments  `json:"grantedAudience"`
}

func NewRequest() *Request {
	return &Request{
		Client:            &DefaultClient{},
		RequestedScope:    Arguments{},
		RequestedAudience: Arguments{},
		GrantedAudience:   Arguments{},
		GrantedScope:      Arguments{},
		Form:              url.Values{},
		RequestedAt:       time.Now().UTC(),
	}
}

func (a *Request) GetID() string {
	if a.ID == "" {
		a.ID = uuid.New()
	}
	return a.ID
}

func (a *Request) SetID(id string) {
	a.ID = id
}

func (a *Request) GetRequestForm() url.Values {
	return a.Form
}

func (a *Request) GetRequestedAt() time.Time {
	return a.RequestedAt
}

func (a *Request) GetClient() Client {
	return a.Client
}

func (a *Request) GetRequestedScopes() Arguments {
	return a.RequestedScope
}

func (a *Request) SetRequestedScopes(s Arguments) {
	a.RequestedScope = nil
	for _, scope := range s {
		a.AppendRequestedScope(scope)
	}
}

func (a *Request) SetRequestedAudience(s Arguments) {
	a.RequestedAudience = nil
	for _, scope := range s {
		a.AppendRequestedAudience(scope)
	}
}

func (a *Request) AppendRequestedScope(scope string) {
	for _, has := range a.RequestedScope {
		if scope == has {
			return
		}
	}
	a.RequestedScope = append(a.RequestedScope, scope)
}

func (a *Request) AppendRequestedAudience(audience string) {
	for _, has := range a.RequestedAudience {
		if audience == has {
			return
		}
	}
	a.RequestedAudience = append(a.RequestedAudience, audience)
}

func (a *Request) GetRequestedAudience() (audience Arguments) {
	return a.RequestedAudience
}

func (a *Request) GrantAudience(audience string) {
	for _, has := range a.GrantedAudience {
		if audience == has {
			return
		}
	}
	a.GrantedAudience = append(a.GrantedAudience, audience)
}

func (a *Request) GetGrantedScopes() Arguments {
	return a.GrantedScope
}

func (a *Request) GetGrantedAudience() Arguments {
	return a.GrantedAudience
}

func (a *Request) GrantScope(scope string) {
	for _, has := range a.GrantedScope {
		if scope == has {
			return
		}
	}
	a.GrantedScope = append(a.GrantedScope, scope)
}

func (a *Request) SetSession(session Session) {
	a.Session = session
}

func (a *Request) GetSession() Session {
	return a.Session
}

func (a *Request) Merge(request Requester) {
	for _, scope := range request.GetRequestedScopes() {
		a.AppendRequestedScope(scope)
	}
	for _, scope := range request.GetGrantedScopes() {
		a.GrantScope(scope)
	}

	for _, aud := range request.GetRequestedAudience() {
		a.AppendRequestedAudience(aud)
	}
	for _, aud := range request.GetGrantedAudience() {
		a.GrantAudience(aud)
	}

	a.RequestedAt = request.GetRequestedAt()
	a.Client = request.GetClient()
	a.Session = request.GetSession()

	for k, v := range request.GetRequestForm() {
		a.Form[k] = v
	}
}

func (a *Request) Sanitize(allowedParameters []string) Requester {
	b := new(Request)
	allowed := map[string]bool{}
	for _, v := range allowedParameters {
		allowed[v] = true
	}

	*b = *a
	b.ID = a.GetID()
	b.Form = url.Values{}
	for k := range a.Form {
		if _, ok := allowed[k]; ok {
			b.Form.Add(k, a.Form.Get(k))
		}
	}

	return b
}
