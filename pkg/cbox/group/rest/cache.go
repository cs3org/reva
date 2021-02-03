// Copyright 2018-2021 CERN
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

package rest

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	grouppb "github.com/cs3org/go-cs3apis/cs3/identity/group/v1beta1"
	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/gomodule/redigo/redis"
)

const (
	groupPrefix           = "group:"
	groupMembersPrefix    = "members:"
	groupInternalIDPrefix = "internal:"
)

func initRedisPool(address, username, password string) *redis.Pool {
	return &redis.Pool{

		MaxIdle:     50,
		MaxActive:   1000,
		IdleTimeout: 240 * time.Second,

		Dial: func() (redis.Conn, error) {
			var c redis.Conn
			var err error
			switch {
			case username != "":
				c, err = redis.Dial("tcp", address,
					redis.DialUsername(username),
					redis.DialPassword(password),
				)
			case password != "":
				c, err = redis.Dial("tcp", address,
					redis.DialPassword(password),
				)
			default:
				c, err = redis.Dial("tcp", address)
			}

			if err != nil {
				return nil, err
			}
			return c, err
		},

		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
}

func (m *manager) setVal(key, val string, expiration int) error {
	conn := m.redisPool.Get()
	defer conn.Close()
	if conn != nil {
		if expiration != -1 {
			if _, err := conn.Do("SET", key, val, "EX", expiration); err != nil {
				return err
			}
		} else {
			if _, err := conn.Do("SET", key, val); err != nil {
				return err
			}
		}
		return nil
	}
	return errors.New("rest: unable to get connection from redis pool")
}

func (m *manager) getVal(key string) (string, error) {
	conn := m.redisPool.Get()
	defer conn.Close()
	if conn != nil {
		val, err := redis.String(conn.Do("GET", key))
		if err != nil {
			return "", err
		}
		return val, nil
	}
	return "", errors.New("rest: unable to get connection from redis pool")
}

func (m *manager) fetchCachedInternalID(gid *grouppb.GroupId) (string, error) {
	return m.getVal(groupPrefix + groupInternalIDPrefix + gid.OpaqueId)
}

func (m *manager) cacheInternalID(gid *grouppb.GroupId, internalID string) error {
	return m.setVal(groupPrefix+groupInternalIDPrefix+gid.OpaqueId, internalID, -1)
}

func (m *manager) fetchCachedGroupDetails(gid *grouppb.GroupId) (*grouppb.Group, error) {
	group, err := m.getVal(groupPrefix + gid.OpaqueId)
	if err != nil {
		return nil, err
	}

	g := grouppb.Group{}
	if err = json.Unmarshal([]byte(group), &g); err != nil {
		return nil, err
	}
	return &g, nil
}

func (m *manager) cacheGroupDetails(g *grouppb.Group) error {
	encodedGroup, err := json.Marshal(&g)
	if err != nil {
		return err
	}
	if err = m.setVal(groupPrefix+g.Id.OpaqueId, string(encodedGroup), -1); err != nil {
		return err
	}

	if err = m.setVal(groupPrefix+"gid_number:"+strconv.FormatInt(g.GidNumber, 10), g.Id.OpaqueId, -1); err != nil {
		return err
	}
	if err = m.setVal(groupPrefix+"mail:"+g.Mail, g.Id.OpaqueId, -1); err != nil {
		return err
	}
	if err = m.setVal(groupPrefix+"group_name:"+g.GroupName, g.Id.OpaqueId, -1); err != nil {
		return err
	}
	return nil
}

func (m *manager) fetchCachedParam(field, claim string) (string, error) {
	return m.getVal(groupPrefix + field + ":" + claim)
}

func (m *manager) fetchCachedGroupMembers(gid *grouppb.GroupId) ([]*userpb.UserId, error) {
	members, err := m.getVal(groupPrefix + groupMembersPrefix + gid.OpaqueId)
	if err != nil {
		return nil, err
	}
	u := []*userpb.UserId{}
	if err = json.Unmarshal([]byte(members), &u); err != nil {
		return nil, err
	}
	return u, nil
}

func (m *manager) cacheGroupMembers(gid *grouppb.GroupId, members []*userpb.UserId) error {
	u, err := json.Marshal(&members)
	if err != nil {
		return err
	}
	if err = m.setVal(groupPrefix+groupMembersPrefix+gid.OpaqueId, string(u), m.conf.GroupMembersCacheExpiration*60); err != nil {
		return err
	}
	return nil
}
