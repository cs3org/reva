-- Copyright 2018-2023 CERN
--
-- Licensed under the Apache License, Version 2.0 (the "License");
-- you may not use this file except in compliance with the License.
-- You may obtain a copy of the License at
--
--     http://www.apache.org/licenses/LICENSE-2.0
--
-- Unless required by applicable law or agreed to in writing, software
-- distributed under the License is distributed on an "AS IS" BASIS,
-- WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
-- See the License for the specific language governing permissions and
-- limitations under the License.
--
-- In applying this license, CERN does not waive the privileges and immunities
-- granted to it by virtue of its status as an Intergovernmental Organization
-- or submit itself to any jurisdiction.

-- This file can be used to make the required changes to the MySQL DB. This is
-- not a proper migration but it should work on most situations.

CREATE TABLE IF NOT EXISTS `routing` (
  `path`       VARCHAR(3072) NOT NULL,
  `mount_id`   VARCHAR(255),
  `mount_type` VARCHAR(100),
  PRIMARY KEY (path)
)

COMMIT;
