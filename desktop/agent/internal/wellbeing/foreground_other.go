//go:build !windows

package wellbeing

func SampleForegroundApp(idleThresholdMs uint64) (exeName string, isIdle bool) {
	return "", false
}
