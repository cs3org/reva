@api
Feature: checksums

  Background:
    Given user "Alice" has been created with default attributes and without skeleton files

  @issue-ocis-reva-99 @skipOnOcis-OCIS-Storage
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario: Upload a file where checksum does not match (new DAV path)
    Given using new DAV path
    When user "Alice" uploads file with checksum "SHA1:f005ba11" and content "Some Text" to "/chksumtst.txt" using the WebDAV API
    Then the HTTP status code should be "201"
    And user "Alice" should see the following elements
      | /chksumtst.txt |