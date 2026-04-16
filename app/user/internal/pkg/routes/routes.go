package routes

import (
	"common/constants"
	"net/url"
)

var (
	masterPing         *url.URL
	masterUserActivate *url.URL
	masterUserReset    *url.URL
	masterHome         *url.URL
)

func init() {
	masterPing, _ = url.Parse(constants.UserAPIPrefix + "/site/ping")
	masterUserActivate, _ = url.Parse("/session/activate")
	masterUserReset, _ = url.Parse("/session/reset")
}
func MasterUserActivateAPIUrl(base *url.URL, uid string) *url.URL {
	route, _ := url.Parse(constants.UserAPIPrefix + "/user/activate/" + uid)
	return base.ResolveReference(route)
}

func MasterUserActivateUrl(base *url.URL) *url.URL {
	return base.ResolveReference(masterUserActivate)
}

func MasterUserResetUrl(base *url.URL) *url.URL {
	return base.ResolveReference(masterUserReset)
}
