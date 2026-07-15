package app

const (
	pbtAPMSuspend         uint32 = 0x0004
	pbtAPMResumeSuspend   uint32 = 0x0007
	pbtAPMResumeAutomatic uint32 = 0x0012
	pbtPowerSettingChange uint32 = 0x8013
)

func powerEventName(event uint32) string {
	switch event {
	case pbtAPMSuspend:
		return "suspend"
	case pbtAPMResumeSuspend:
		return "resume-user"
	case pbtAPMResumeAutomatic:
		return "resume-automatic"
	case pbtPowerSettingChange:
		return "power-setting-change"
	default:
		return "unknown"
	}
}

func isResumePowerEvent(event uint32) bool {
	return event == pbtAPMResumeSuspend || event == pbtAPMResumeAutomatic
}
