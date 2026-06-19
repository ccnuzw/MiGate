package web

import "net/http"

func ExposeForTestCoreInstallHandler(core string, runner func(script string) ([]byte, error)) http.HandlerFunc {
	return coreInstallHandler(core, runner)
}
