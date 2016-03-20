package yammer

import (
	"github.com/masahide/ipmes2yammer/oauth"
)

type Yammer struct {
	transport *oauth.Transport
	config    *oauth.Config
	lsConfig  *LocalServerConfig
}

type LocalServerConfig struct {
	Port    int
	Timeout int
}

type RedirectResult struct {
	Code string
	Err  error
}

func NewYammer(lsConfig *LocalServerConfig) *Yammer {
	yammer := Yammer{lsConfig: lsConfig}
	return &yammer
}
