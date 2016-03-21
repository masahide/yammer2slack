package yammer

import (
	"github.com/masahide/yammer2slack/oauth"
)

// Yammer struct
type Yammer struct {
	transport *oauth.Transport
	config    *oauth.Config
	lsConfig  *LocalServerConfig
}

// LocalServerConfig config struct
type LocalServerConfig struct {
	Port    int
	Timeout int
}

// RedirectResult redirect result struct
type RedirectResult struct {
	Code string
	Err  error
}

// NewYammer create Yammer struct
func NewYammer(lsConfig *LocalServerConfig) *Yammer {
	yammer := Yammer{lsConfig: lsConfig}
	return &yammer
}
