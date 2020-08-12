@api @files_sharing-app-required @public_link_share-feature-required @issue-ocis-reva-252
Feature: update a public link share

  Background:
    Given using OCS API version "1"
    And user "Alice" has been created with default attributes and skeleton files

  @issue-ocis-reva-243 @issue-ocis-reva-349 @issue-37653
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario Outline: API responds with a full set of parameters when owner changes the expireDate of a public share
    Given using OCS API version "<ocs_api_version>"
    When user "Alice" creates a public link share using the sharing API with settings
      | path | FOLDER |
    And user "Alice" updates the last share using the sharing API with
      | expireDate | +3 days |
    Then the OCS status code should be "<ocs_status_code>"
    And the OCS status message should be "OK"
    And the HTTP status code should be "200"
    And the fields of the last response to user "Alice" should include
      | id                         | A_STRING             |
      | share_type                 | public_link          |
      | uid_owner                  | %username%           |
      | displayname_owner          | %displayname%        |
      | permissions                | read                 |
      | stime                      | A_NUMBER             |
      | parent                     |                      |
      | expiration                 | A_STRING             |
      | token                      | A_STRING             |
      | uid_file_owner             | %username%           |
      | displayname_file_owner     | %displayname%        |
      | additional_info_owner      |                      |
      | additional_info_file_owner |                      |
      | state                      | 0                    |
      | item_type                  | folder               |
      | item_source                | A_STRING             |
      | path                       | /FOLDER              |
      | mimetype                   | httpd/unix-directory |
      | storage_id                 | A_STRING             |
      | storage                    | A_NUMBER             |
      | file_source                | A_STRING             |
      | file_target                | /FOLDER              |
      | mail_send                  | 0                    |
      | name                       |                      |
    Examples:
      | ocs_api_version | ocs_status_code |
      | 1               | 100             |
      | 2               | 200             |
