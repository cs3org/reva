@api
Feature: get file properties
  As a user
  I want to be able to get meta-information about files
  So that I can know file meta-information (detailed requirement TBD)

  Background:
    Given using OCS API version "1"
    And user "Alice" has been created with default attributes and without skeleton files

  @skipOnOcis-OC-Storage @issue-ocis-reva-265 @skipOnOcis-OCIS-Storage
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario Outline: upload a file to content
    Given using <dav_version> DAV path
    When user "Alice" uploads file with content "uploaded content" to "<file_name>" using the WebDAV API
    Then the HTTP status code should be "500"
    Examples:
      | dav_version | file_name     |
      | old         | /file ?2.txt  |
      | new         | /file ?2.txt  |

  @skipOnOcis-OC-Storage @issue-ocis-reva-265 @skipOnOcis-OCIS-Storage
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario Outline: Do a PROPFIND of various folder names
    Given using <dav_version> DAV path
    And user "Alice" has created folder "/folder ?2.txt"
    When user "Alice" uploads to these filenames with content "uploaded content" using the webDAV API then the results should be as listed
      | filename                 | http-code | exists |
      | /folder ?2.txt/file1.txt | 500       | no     |
    Examples:
      | dav_version |
      | old         |
      | new         |

  @issue-ocis-reva-163
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario Outline: Do a PROPFIND to a non-existing URL
    And user "Alice" requests "<url>" with "PROPFIND" using basic auth
    Then the body of the response should be empty
    Examples:
      | url                                  |
      | /remote.php/dav/files/does-not-exist |
      | /remote.php/dav/does-not-exist       |
