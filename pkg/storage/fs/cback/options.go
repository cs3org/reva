package cback

// Options for the CBACK module
type Options struct {
	ImpersonatorToken string `mapstructure:"impersonator"`
	ApiURL            string `mapstructure:"apiURL"`
}
