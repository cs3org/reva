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

CREATE TABLE `notifications` (
	`id` INT PRIMARY KEY AUTO_INCREMENT,
	`ref` VARCHAR(3072) UNIQUE NOT NULL,
	`template_name` VARCHAR(320) NOT NULL
);

COMMIT;

CREATE TABLE `notification_recipients` (
	`id` INT PRIMARY KEY AUTO_INCREMENT,
	`notification_id` INT NOT NULL,
	`recipient` VARCHAR(320) NOT NULL,
	FOREIGN KEY (notification_id)
		REFERENCES notifications (id)
		ON DELETE CASCADE
);

COMMIT;

CREATE INDEX `notifications_ix0` ON `notifications` (`ref`);

CREATE INDEX `notification_recipients_ix0` ON `notification_recipients` (`notification_id`);
CREATE INDEX `notification_recipients_ix1` ON `notification_recipients` (`recipient`);

-- changes for added notifications on oc shares

ALTER TABLE oc_share ADD notify_uploads INTEGER DEFAULT 0 NOT NULL;
ALTER TABLE oc_share ADD notify_uploads_extra_recipients VARCHAR(2048);
