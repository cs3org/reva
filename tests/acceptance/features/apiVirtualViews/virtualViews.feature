@api @virtual-views-required
Feature: virtual views
  As admin
  I want to be able to shard large folders over multiple storage providers
  So that I can scale large numbers of users better.

  Background:
    Given user "einstein" deletes everything from folder "virtual/" using the WebDAV API
    And user "einstein" has created the following folders
      | path            |
      | virtual/a       |
      | virtual/a/alice |
      | virtual/b       |
      | virtual/c       |
      | virtual/k       |
      | virtual/l       |
      | virtual/z       |

  Scenario: list large folder
    Given using old DAV path
    When user "einstein" lists the resources in "/virtual" with depth "0" using the WebDAV API
    Then the HTTP status code should be "207"
    And the last DAV response for user "einstein" should not contain these nodes
      | name      |
      | virtual/a |
      | virtual/b |
      | virtual/c |
      | virtual/k |
      | virtual/l |
      | virtual/z |
    When user "einstein" lists the resources in "/virtual" with depth 1 using the WebDAV API
    Then the HTTP status code should be "207"
    And the last DAV response for user "einstein" should contain these nodes
      | name |
      | virtual/a    |
      | virtual/b    |
      | virtual/c    |
      | virtual/k    |
      | virtual/l    |
      | virtual/z    |
    And the last DAV response for user "einstein" should not contain these nodes
      | name            |
      | virtual/a/alice |
    When user "einstein" lists the resources in "/virtual" with depth "infinity" using the WebDAV API
    Then the HTTP status code should be "207"
    And the last DAV response for user "einstein" should contain these nodes
      | name            |
      | virtual/a       |
      | virtual/a/alice |
      | virtual/b       |
      | virtual/c       |
      | virtual/k       |
      | virtual/l       |
      | virtual/z       |

  Scenario: etag changes when adding a folder
    Given user "einstein" has stored etag of element "/"
    And user "einstein" has stored etag of element "/virtual"
    And user "einstein" has stored etag of element "/virtual/b"
    When user "einstein" creates folder "/virtual/b/bar" using the WebDAV API
    Then these etags should have changed:
      | user     | path       |
      | einstein | /          |
      | einstein | /virtual   |
      | einstein | /virtual/b |