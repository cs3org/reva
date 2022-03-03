Bugfix: pass spacegrants when adding member to space

When creating a space grant there should not be created a new space. 
Unfortunately SpaceGrant didn't work when adding members to a space.
Now a value is placed in the ctx of the storageprovider on which decomposedfs reacts



https://github.com/cs3org/reva/pull/2464
