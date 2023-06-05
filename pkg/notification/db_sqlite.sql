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

-- This file can be used to quickstart a SQLite DB for running the tests in
-- ./manager/sql/sql_test.go

CREATE TABLE `cbox_notifications` (
        `id` INTEGER PRIMARY KEY AUTOINCREMENT,
        `ref` VARCHAR(3072) UNIQUE NOT NULL,
        `template_name` VARCHAR(320) NOT NULL
);

COMMIT;

CREATE TABLE `cbox_notification_recipients` (
	`id` INTEGER PRIMARY KEY AUTOINCREMENT,
	`notification_id` INTEGER NOT NULL,
	`recipient` VARCHAR(320) NOT NULL,
	FOREIGN KEY (notification_id)
		REFERENCES cbox_notifications (id)
		ON DELETE CASCADE
);

COMMIT;

CREATE INDEX `cbox_notifications_ix0` ON `cbox_notifications` (`ref`);

CREATE INDEX `cbox_notification_recipients_ix0` ON `cbox_notification_recipients` (`notification_id`);
CREATE INDEX `cbox_notification_recipients_ix1` ON `cbox_notification_recipients` (`recipient`);

COMMIT;

INSERT INTO `cbox_notifications` (`id`, `ref`, `template_name`) VALUES (1, "notification-test", "notification-template-test");
INSERT INTO `cbox_notification_recipients` (`id`, `notification_id`, `recipient`) VALUES (1, 1, "jdoe"), (2, 1, "testuser");

COMMIT;
