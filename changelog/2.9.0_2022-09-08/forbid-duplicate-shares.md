Bugfix: Forbid duplicate shares

When sending a CreateShare request twice two shares would be created, one being not accessible.
This was blocked by web so the issue wasn't obvious. Now it's forbidden to create share for a
user who already has a share on that same resource

https://github.com/cs3org/reva/pull/3176
