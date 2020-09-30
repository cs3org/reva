@api @issue-ocis-187
Feature: previews of files downloaded through the webdav API

  Background:
    Given user "Alice" has been created with default attributes and without skeleton files

  @issue-ocis-188
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario Outline: download previews with invalid width
    Given user "Alice" has uploaded file "filesForUpload/lorem.txt" to "/parent.txt"
    When user "Alice" downloads the preview of "/parent.txt" with width "<width>" and height "32" using the WebDAV API
    Then the HTTP status code should be "200"
    Examples:
      | width |
      | 0     |
      | 0.5   |
      | -1    |
      | false |
      | true  |
      | A     |
      | %2F   |

  @issue-ocis-188
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario Outline: download previews with invalid height
    Given user "Alice" has uploaded file "filesForUpload/lorem.txt" to "/parent.txt"
    When user "Alice" downloads the preview of "/parent.txt" with width "32" and height "<height>" using the WebDAV API
    Then the HTTP status code should be "200"
    Examples:
      | height |
      | 0      |
      | 0.5    |
      | -1     |
      | false  |
      | true   |
      | A      |
      | %2F    |

  @issue-ocis-189
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario Outline: download previews of file types that don't support preview
    Given user "Alice" has uploaded file "filesForUpload/<filename>" to "/<newfilename>"
    When user "Alice" downloads the preview of "/<newfilename>" with width "32" and height "32" using the WebDAV API
    Then the HTTP status code should be "200"
    Examples:
      | filename     | newfilename |
      | simple.pdf   | test.pdf    |
      | simple.odt   | test.odt    |
      | new-data.zip | test.zip    |

  @issue-ocis-187
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario Outline: download previews of different image file types
    Given user "Alice" has uploaded file "filesForUpload/<imageName>" to "/<newImageName>"
    When user "Alice" downloads the preview of "/<newImageName>" with width "32" and height "32" using the WebDAV API
    Then the HTTP status code should be "200"
    And the downloaded image should be "1240" pixels wide and "648" pixels high
    Examples:
      | imageName      | newImageName  |
      | testavatar.jpg | testimage.jpg |
      | testavatar.png | testimage.png |

  @issue-ocis-187
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario: download previews of image after renaming it
    Given user "Alice" has uploaded file "filesForUpload/testavatar.jpg" to "/testimage.jpg"
    When user "Alice" moves file "/testimage.jpg" to "/testimage.txt" using the WebDAV API
    And user "Alice" downloads the preview of "/testimage.txt" with width "32" and height "32" using the WebDAV API
    Then the HTTP status code should be "200"
    And the downloaded image should be "1240" pixels wide and "648" pixels high

  @issue-ocis-190
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario: download previews of folders
    Given user "Alice" has created folder "subfolder"
    When user "Alice" downloads the preview of "/subfolder/" with width "32" and height "32" using the WebDAV API
    Then the HTTP status code should be "501"

  @issue-ocis-192
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario: Download file previews when it is disabled by the administrator
    Given the administrator has updated system config key "enable_previews" with value "false" and type "boolean"
    And user "Alice" has uploaded file "filesForUpload/lorem.txt" to "/parent.txt"
    When user "Alice" downloads the preview of "/parent.txt" with width "32" and height "32" using the WebDAV API
    Then the HTTP status code should be "200"

  @issue-ocis-193
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario: unset maximum size of previews
    Given user "Alice" has uploaded file "filesForUpload/lorem.txt" to "/parent.txt"
    And the administrator has updated system config key "preview_max_x" with value "null"
    And the administrator has updated system config key "preview_max_y" with value "null"
    When user "Alice" downloads the preview of "/parent.txt" with width "32" and height "32" using the WebDAV API
    Then the HTTP status code should be "200"

  @issue-ocis-193 @skipOnOcis-OCIS-Storage
  # after fixing all issues delete this Scenario and use the one from oC10 core
  Scenario: set maximum size of previews
    Given user "Alice" has uploaded file "filesForUpload/lorem.txt" to "/parent.txt"
    When the administrator updates system config key "preview_max_x" with value "null" using the occ command
    And the administrator updates system config key "preview_max_y" with value "null" using the occ command
    Then the HTTP status code should be "201"
    When user "Alice" downloads the preview of "/parent.txt" with width "null" and height "null" using the WebDAV API
    Then the HTTP status code should be "200"
