@api @issue-ocis-reva-14
Feature: users cannot move (rename) a file to a blacklisted name
  As an administrator
  I want to be able to prevent users from moving (renaming) files to specified file names
  So that I can prevent unwanted file names existing in the cloud storage

  Background:
    Given using OCS API version "1"
    And user "Alice" has been created with default attributes and skeleton files

  @issue-ocis-reva-211
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario Outline: rename a file to a filename that is banned by default
    Given using <dav_version> DAV path
    When user "Alice" moves file "/welcome.txt" to "/.htaccess" using the WebDAV API
    Then the HTTP status code should be "201"
    And as "Alice" file "/.htaccess" should exist
    Examples:
      | dav_version |
      | old         |
      | new         |
