Enhancement: add checks when removing space members

- Removed owners from project spaces
- Prevent deletion of last space manager
- Viewers and editors can always be deleted
- Managers can only be deleted when there will be at least one remaining

https://github.com/cs3org/reva/pull/2524
