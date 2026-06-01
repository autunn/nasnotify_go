package app

const (
	AppID        = "com.autunn.nasnotifyfresh"
	AppProxyPath = "nasnotify"
)

func AppRoutePrefixes() []string {
	return []string{
		"/",
		"/" + AppProxyPath,
		"/" + AppID,
		"/ugreen/:ugVersion/" + AppProxyPath,
		"/ugreen/:ugVersion/" + AppID,
	}
}
