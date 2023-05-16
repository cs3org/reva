Enhancement: Cleanup Space Delete permissions

Space Delete and Disable permissions ("Drive.ReadWriteEnabled", "delete-all-spaces", "delete-all-home-spaces") were overlapping and not clear differentiatable.
The new logic is as follows:
  -  "Drive.ReadWriteEnabled" allows enabling or disabling a project space
  -  "delete-all-home-spaces" allows deleting personal spaces of users
  -  "delete-all-spaces" allows deleting a project space
  -  Space Mangers can still disable/enable a drive

https://github.com/cs3org/reva/pull/3893
