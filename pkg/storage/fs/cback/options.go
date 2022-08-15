package cback

// Options for the CBACK module
type Options struct {
	ImpersonatorToken string `mapstructure:"token"`
	APIURL            string `mapstructure:"api_url"`
}
