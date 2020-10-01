package ocis

// Option defines a single option function.
type Option func(o *Options)

// Options defines the available options for this package.
type Options struct {
	// ocis fs works on top of a dir of uuid nodes
	Root string `mapstructure:"root"`

	// UserLayout describes the relative path from the storage's root node to the users home node.
	UserLayout string `mapstructure:"user_layout"`

	// TODO NodeLayout option to save nodes as eg. nodes/1d/d8/1dd84abf-9466-4e14-bb86-02fc4ea3abcf
	ShareFolder string `mapstructure:"share_folder"`

	// EnableHome enables the creation of home directories.
	EnableHome bool `mapstructure:"enable_home"`

	// propagate mtime changes as tmtime (tree modification time) to the parent directory when user.ocis.propagation=1 is set on a node
	TreeTimeAccounting bool `mapstructure:"treetime_accounting"`

	// propagate size changes as treesize
	TreeSizeAccounting bool `mapstructure:"treesize_accounting"`
}

// newOptions initializes the available default options.
func newOptions(opts ...Option) Options {
	opt := Options{}

	for _, o := range opts {
		o(&opt)
	}

	return opt
}

// Name provides a function to set the root option.
func Root(val string) Option {
	return func(o *Options) {
		o.Root = val
	}
}

// UserLayout provides a function to set the user layout option.
func UserLayout(val string) Option {
	return func(o *Options) {
		o.UserLayout = val
	}
}

// ShareFolder provides a function to set the ShareFolder option.
func ShareFolder(val string) Option {
	return func(o *Options) {
		o.ShareFolder = val
	}
}

// EnableHome provides a function to set the EnableHome option.
func EnableHome(val bool) Option {
	return func(o *Options) {
		o.EnableHome = val
	}
}

// TreeTimeAccounting provides a function to set the TreeTimeAccounting option.
func TreeTimeAccounting(val bool) Option {
	return func(o *Options) {
		o.TreeTimeAccounting = val
	}
}

// TreeSizeAccounting provides a function to set the TreeSizeAccounting option.
func TreeSizeAccounting(val bool) Option {
	return func(o *Options) {
		o.TreeSizeAccounting = val
	}
}
