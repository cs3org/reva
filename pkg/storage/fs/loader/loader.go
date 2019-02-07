package loader

import (
	// Load core storage filesystem backends.
	_ "github.com/cernbox/reva/pkg/storage/fs/eos"
	_ "github.com/cernbox/reva/pkg/storage/fs/local"
	// Add your own here
)
