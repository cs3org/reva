package cback

// Options for the CBACK module
type Options struct {
	ImpersonatorToken string `mapstructure:"impersonator"`
	APIURL            string `mapstructure:"apiURL"`
}
