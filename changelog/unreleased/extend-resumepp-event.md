Enhancement: Extend ResumePostprocessing event

Instead of just sending an uploadID, one can set a postprocessing step now to restart all uploads in this step
Also adds a new postprocessing step - "finished" - which means that postprocessing is finished but the storage provider
hasn't acknowledged it yet.

https://github.com/cs3org/reva/pull/4477
