@api @files_versions-app-required @skipOnOcis-EOS-Storage @issue-ocis-reva-275

Feature: dav-versions

  Background:
    Given using OCS API version "2"
    And using new DAV path
    And user "Alice" has been created with default attributes and without skeleton files

  @issue-ocis-reva-17 @issue-ocis-reva-56
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario: Upload file and no version is available using various chunking methods
    When user "Alice" uploads file "filesForUpload/davtest.txt" to filenames based on "/davtest.txt" with all mechanisms using the WebDAV API
    Then the version folder of file "/davtest.txt-olddav-regular" for user "Alice" should contain "0" elements
    And the version folder of file "/davtest.txt-newdav-regular" for user "Alice" should contain "0" elements
    And the version folder of file "/davtest.txt-olddav-oldchunking" for user "Alice" should contain "0" elements
    And as "Alice" file "/davtest.txt-newdav-newchunking" should not exist

  @issue-ocis-reva-17 @issue-ocis-reva-56 @skipOnOcis-OCIS-Storage
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario: Upload a file twice and versions are available using various chunking methods
    When user "Alice" uploads file "filesForUpload/davtest.txt" to filenames based on "/davtest.txt" with all mechanisms using the WebDAV API
    And user "Alice" uploads file "filesForUpload/davtest.txt" to filenames based on "/davtest.txt" with all mechanisms using the WebDAV API
    Then the version folder of file "/davtest.txt-olddav-regular" for user "Alice" should contain "1" element
    And the version folder of file "/davtest.txt-newdav-regular" for user "Alice" should contain "1" element
    And the version folder of file "/davtest.txt-olddav-oldchunking" for user "Alice" should contain "1" element
    And as "Alice" file "/davtest.txt-newdav-newchunking" should not exist

  @files_sharing-app-required
  @issue-ocis-reva-243
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario: sharer of a file can see the old version information when the sharee changes the content of the file
    Given user "Brian" has been created with default attributes and without skeleton files
    And user "Alice" has uploaded file with content "First content" to "sharefile.txt"
    And user "Alice" has shared file "sharefile.txt" with user "Brian"
    When user "Brian" has uploaded file with content "Second content" to "/sharefile.txt"
    Then the HTTP status code should be "201"
    And the version folder of file "/sharefile.txt" for user "Alice" should contain "0" element
#    And the version folder of file "/sharefile.txt" for user "Alice" should contain "1" element

  @files_sharing-app-required
  @issue-ocis-reva-243 @skipOnOcis-OCIS-Storage
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario: sharer of a file can restore the original content of a shared file after the file has been modified by the sharee
    Given user "Brian" has been created with default attributes and without skeleton files
    And user "Alice" has uploaded file with content "First content" to "sharefile.txt"
    And user "Alice" has shared file "sharefile.txt" with user "Brian"
    And user "Brian" has uploaded file with content "Second content" to "/sharefile.txt"
    When user "Alice" restores version index "0" of file "/sharefile.txt" using the WebDAV API
#    When user "Alice" restores version index "1" of file "/sharefile.txt" using the WebDAV API
    Then the HTTP status code should be "201"
    And the content of file "/sharefile.txt" for user "Alice" should be "First content"
    And the content of file "/sharefile.txt" for user "Brian" should be "Second content"
#    And the content of file "/sharefile.txt" for user "Brian" should be "First content"

  @files_sharing-app-required
  @issue-ocis-reva-243 @issue-ocis-reva-386
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario Outline: Moving a file (with versions) into a shared folder as the sharee and as the sharer (old DAV path)
    Given using <dav_version> DAV path
    And user "Brian" has been created with default attributes and without skeleton files
    And user "Brian" has created folder "/testshare"
    And user "Brian" has created a share with settings
      | path        | testshare |
      | shareType   | user      |
      | permissions | change    |
      | shareWith   | Alice     |
    And user "Brian" has uploaded file with content "test data 1" to "/testfile.txt"
    And user "Brian" has uploaded file with content "test data 2" to "/testfile.txt"
    And user "Brian" has uploaded file with content "test data 3" to "/testfile.txt"
    And user "Brian" moves file "/testfile.txt" to "/testshare/testfile.txt" using the WebDAV API
    Then the HTTP status code should be "201"
    And the content of file "/testshare/testfile.txt" for user "Alice" should be ""
#    And the content of file "/testshare/testfile.txt" for user "Alice" should be "test data 3"
    And the content of file "/testshare/testfile.txt" for user "Brian" should be "test data 3"
    And as "Brian" file "/testfile.txt" should not exist
    And as "Alice" file "/testshare/testfile.txt" should not exist
    And the content of file "/testshare/testfile.txt" for user "Brian" should be "test data 3"
#    And the version folder of file "/testshare/testfile.txt" for user "Alice" should contain "2" elements
#    And the version folder of file "/testshare/testfile.txt" for user "Brian" should contain "2" elements
    Examples:
      | dav_version |
      | old         |

  @files_sharing-app-required
  @issue-ocis-reva-243 @issue-ocis-reva-386 @skipOnOcis-OCIS-Storage
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario Outline: Moving a file (with versions) into a shared folder as the sharee and as the sharer (new DAV path)
    Given using <dav_version> DAV path
    And user "Brian" has been created with default attributes and without skeleton files
    And user "Brian" has created folder "/testshare"
    And user "Brian" has created a share with settings
      | path        | testshare |
      | shareType   | user      |
      | permissions | change    |
      | shareWith   | Alice     |
    And user "Brian" has uploaded file with content "test data 1" to "/testfile.txt"
    And user "Brian" has uploaded file with content "test data 2" to "/testfile.txt"
    And user "Brian" has uploaded file with content "test data 3" to "/testfile.txt"
    And user "Brian" moves file "/testfile.txt" to "/testshare/testfile.txt" using the WebDAV API
    Then the HTTP status code should be "201"
    And the content of file "/testshare/testfile.txt" for user "Alice" should be ""
#    And the content of file "/testshare/testfile.txt" for user "Alice" should be "test data 3"
    And the content of file "/testshare/testfile.txt" for user "Brian" should be "test data 3"
    And as "Brian" file "/testfile.txt" should not exist
    And as "Alice" file "/testshare/testfile.txt" should not exist
    And the content of file "/testshare/testfile.txt" for user "Brian" should be "test data 3"
#    And the version folder of file "/testshare/testfile.txt" for user "Alice" should contain "2" elements
#    And the version folder of file "/testshare/testfile.txt" for user "Brian" should contain "2" elements
    Examples:
      | dav_version |
      | new         |

  @files_sharing-app-required
  @issue-ocis-reva-243 @issue-ocis-reva-386 @skipOnOcis-OCIS-Storage
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario Outline: Moving a file (with versions) out of a shared folder as the sharee and as the sharer
    Given using <dav_version> DAV path
    And user "Brian" has been created with default attributes and without skeleton files
    And user "Brian" has created folder "/testshare"
    And user "Brian" has uploaded file with content "test data 1" to "/testshare/testfile.txt"
    And user "Brian" has uploaded file with content "test data 2" to "/testshare/testfile.txt"
    And user "Brian" has uploaded file with content "test data 3" to "/testshare/testfile.txt"
    And user "Brian" has created a share with settings
      | path        | testshare |
      | shareType   | user      |
      | permissions | change    |
      | shareWith   | Alice     |
    When user "Brian" moves file "/testshare/testfile.txt" to "/testfile.txt" using the WebDAV API
    Then the HTTP status code should be "201"
    And the content of file "/testfile.txt" for user "Brian" should be "test data 3"
    And as "Alice" file "/testshare/testfile.txt" should not exist
    And as "Brian" file "/testshare/testfile.txt" should not exist
#    And the version folder of file "/testfile.txt" for user "Brian" should contain "2" elements
    Examples:
      | dav_version |
      | old         |
      | new         |
