package cback

type Options struct {
	Root              string `mapstructure:"root"`
	ImpersonatorToken string `mapstructure:"impersonator"`
	API_Url           string `mapstructure:"api_url"`
}
