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

package rest

import (
	"encoding/json"
	"errors"
	"time"

	userpb "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	"github.com/gomodule/redigo/redis"
)

const (
	userDetailsPrefix = "user:"
	userGroupsPrefix  = "groups:"
)

func initRedisPool(port string) *redis.Pool {
	return &redis.Pool{

		MaxIdle:     50,
		MaxActive:   1000,
		IdleTimeout: 240 * time.Second,

		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", port)
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

func (m *manager) fetchCachedUserDetails(uid *userpb.UserId) (*userpb.User, error) {
	conn := m.redisPool.Get()
	defer conn.Close()
	if conn != nil {
		user, err := redis.String(conn.Do("GET", userDetailsPrefix+uid.OpaqueId))
		if err != nil {
			return nil, err
		}
		u := userpb.User{}
		if err = json.Unmarshal([]byte(user), &u); err != nil {
			return nil, err
		}
		return &u, nil
	}
	return nil, errors.New("rest: unable to get connection from redis pool")
}

func (m *manager) cacheUserDetails(u *userpb.User) error {
	conn := m.redisPool.Get()
	defer conn.Close()
	if conn != nil {
		encodedUser, err := json.Marshal(&u)
		if err != nil {
			return err
		}
		if _, err = conn.Do("SET", userDetailsPrefix+u.Id.OpaqueId, string(encodedUser)); err != nil {
			return err
		}
		return nil
	}
	return errors.New("rest: unable to get connection from redis pool")
}

func (m *manager) fetchCachedUserGroups(uid *userpb.UserId) ([]string, error) {
	conn := m.redisPool.Get()
	defer conn.Close()
	if conn != nil {
		groups, err := redis.String(conn.Do("GET", userGroupsPrefix+uid.OpaqueId))
		if err != nil {
			return nil, err
		}
		g := []string{}
		if err = json.Unmarshal([]byte(groups), &g); err != nil {
			return nil, err
		}
		return g, nil
	}
	return nil, errors.New("rest: unable to get connection from redis pool")
}

func (m *manager) cacheUserGroups(uid *userpb.UserId, groups []string) error {
	conn := m.redisPool.Get()
	defer conn.Close()
	if conn != nil {
		encodedGroups, err := json.Marshal(&groups)
		if err != nil {
			return err
		}
		if _, err = conn.Do("SET", userGroupsPrefix+uid.OpaqueId, string(encodedGroups), "EX", m.conf.UserGroupsCacheExpiration*60); err != nil {
			return err
		}
		return nil
	}
	return errors.New("rest: unable to get connection from redis pool")
}
