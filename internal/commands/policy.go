package commands

type CommandPolicy struct {
	RemoteShell    bool
	RemoteFiles    bool
	AdminOps       bool
	LocalInference bool
	ComputerApps   bool
	ComputerUse    bool
}

func (p CommandPolicy) Allows(kind string) bool {
	switch kind {
	case KindCancel, KindHealth, KindPortDiscover:
		return true
	case KindExec, KindExecStream,
		KindTmuxRun, KindTmuxCapture, KindTmuxSendKeys, KindTmuxList, KindTmuxKill:
		return p.RemoteShell
	case KindFileRead, KindFileWrite, KindFileDelete, KindFileList, KindFileStat,
		KindFileMkdir, KindFileMove, KindFileGrep, KindFileFind,
		KindFileUpload, KindFileDownload:
		return p.RemoteFiles
	case KindUserList, KindUserCreate, KindUserDelete, KindUserEditGroups,
		KindEnvList, KindEnvSet, KindEnvDelete, KindShutdown:
		return p.AdminOps
	case KindLocalStatus, KindLocalInstall, KindLocalUpdate, KindLocalStart,
		KindLocalStop, KindLocalLogs, KindLocalModelList, KindLocalModelPull,
		KindLocalModelRm, KindLocalModelCreate, KindLocalChat:
		return p.LocalInference
	case KindAppRuntimeStatus, KindAppRuntimeSetup, KindAppList, KindAppExternalList,
		KindAppInspect, KindAppCreate, KindAppRun, KindAppComposeUp, KindAppBuild,
		KindAppStart, KindAppStop, KindAppRestart, KindAppDelete, KindAppReset,
		KindAppLogs, KindAppExec, KindAppPorts, KindAppExpose, KindAppUnexpose:
		return p.ComputerApps
	case KindDesktop:
		return p.ComputerUse
	default:
		return false
	}
}
