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
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/gomodule/redigo/redis"
)

const (
	userPrefix           = "user:"
	userGroupsPrefix     = "groups:"
	userInternalIDPrefix = "internal:"
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

func (m *manager) fetchCachedInternalID(uid *userpb.UserId) (string, error) {
	return m.getVal(userPrefix + userInternalIDPrefix + uid.OpaqueId)
}

func (m *manager) cacheInternalID(uid *userpb.UserId, internalID string) error {
	return m.setVal(userPrefix+userInternalIDPrefix+uid.OpaqueId, internalID, -1)
}

func (m *manager) fetchCachedUserDetails(uid *userpb.UserId) (*userpb.User, error) {
	user, err := m.getVal(userPrefix + uid.OpaqueId)
	if err != nil {
		return nil, err
	}

	u := userpb.User{}
	if err = json.Unmarshal([]byte(user), &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (m *manager) cacheUserDetails(u *userpb.User) error {
	encodedUser, err := json.Marshal(&u)
	if err != nil {
		return err
	}
	if err = m.setVal(userPrefix+u.Id.OpaqueId, string(encodedUser), -1); err != nil {
		return err
	}

	uid, err := extractUID(u)
	if err != nil {
		return err
	}

	if err = m.setVal(userPrefix+"uid:"+uid, u.Id.OpaqueId, -1); err != nil {
		return err
	}
	if err = m.setVal(userPrefix+"mail:"+u.Mail, u.Id.OpaqueId, -1); err != nil {
		return err
	}
	if err = m.setVal(userPrefix+"username:"+u.Username, u.Id.OpaqueId, -1); err != nil {
		return err
	}
	return nil
}

func (m *manager) fetchCachedParam(field, claim string) (string, error) {
	return m.getVal(userPrefix + field + ":" + claim)
}

func (m *manager) fetchCachedUserGroups(uid *userpb.UserId) ([]string, error) {
	groups, err := m.getVal(userPrefix + userGroupsPrefix + uid.OpaqueId)
	if err != nil {
		return nil, err
	}
	g := []string{}
	if err = json.Unmarshal([]byte(groups), &g); err != nil {
		return nil, err
	}
	return g, nil
}

func (m *manager) cacheUserGroups(uid *userpb.UserId, groups []string) error {
	g, err := json.Marshal(&groups)
	if err != nil {
		return err
	}
	if err = m.setVal(userPrefix+userGroupsPrefix+uid.OpaqueId, string(g), m.conf.UserGroupsCacheExpiration*60); err != nil {
		return err
	}
	return nil
}
