package commands

import "encoding/json"

const (
	KindExec             = "exec"
	KindExecStream       = "exec-stream"
	KindFileRead         = "file-read"
	KindFileWrite        = "file-write"
	KindFileDelete       = "file-delete"
	KindFileList         = "file-list"
	KindFileStat         = "file-stat"
	KindFileMkdir        = "file-mkdir"
	KindFileMove         = "file-move"
	KindFileGrep         = "file-grep"
	KindFileFind         = "file-find"
	KindFileUpload       = "file-upload"
	KindFileDownload     = "file-download"
	KindTmuxRun          = "tmux-run"
	KindTmuxCapture      = "tmux-capture"
	KindTmuxSendKeys     = "tmux-send-keys"
	KindTmuxList         = "tmux-list"
	KindTmuxKill         = "tmux-kill"
	KindUserList         = "user-list"
	KindUserCreate       = "user-create"
	KindUserDelete       = "user-delete"
	KindUserEditGroups   = "user-edit-groups"
	KindEnvList          = "env-list"
	KindEnvSet           = "env-set"
	KindEnvDelete        = "env-delete"
	KindLocalStatus      = "local-inference-status"
	KindLocalInstall     = "local-inference-runtime-install"
	KindLocalUpdate      = "local-inference-runtime-update"
	KindLocalStart       = "local-inference-runtime-start"
	KindLocalStop        = "local-inference-runtime-stop"
	KindLocalLogs        = "local-inference-runtime-logs"
	KindLocalModelList   = "local-inference-model-list"
	KindLocalModelPull   = "local-inference-model-pull"
	KindLocalModelRm     = "local-inference-model-remove"
	KindLocalModelCreate = "local-inference-model-create"
	KindLocalChat        = "local-inference-chat"
	KindAppRuntimeStatus = "computer-app-runtime-status"
	KindAppRuntimeSetup  = "computer-app-runtime-setup"
	KindAppList          = "computer-app-list"
	KindAppExternalList  = "computer-app-external-list"
	KindAppInspect       = "computer-app-inspect"
	KindAppCreate        = "computer-app-create"
	KindAppRun           = "computer-app-run"
	KindAppComposeUp     = "computer-app-compose-up"
	KindAppBuild         = "computer-app-build"
	KindAppStart         = "computer-app-start"
	KindAppStop          = "computer-app-stop"
	KindAppRestart       = "computer-app-restart"
	KindAppDelete        = "computer-app-delete"
	KindAppReset         = "computer-app-reset"
	KindAppLogs          = "computer-app-logs"
	KindAppExec          = "computer-app-exec"
	KindAppPorts         = "computer-app-ports"
	KindAppExpose        = "computer-app-expose"
	KindAppUnexpose      = "computer-app-unexpose"
	KindDesktop          = "desktop"
	KindCancel           = "cancel"
	KindPortDiscover     = "port-discover"
	KindHealth           = "health"
	KindShutdown         = "shutdown"
)

const (
	ErrInvalidPayload     = "invalid-payload"
	ErrUnsupportedKind    = "unsupported-kind"
	ErrRunAsNotFound      = "runas-not-found"
	ErrRunAsForbidden     = "runas-forbidden"
	ErrCommandTimeout     = "command-timeout"
	ErrOutputTruncated    = "output-truncated"
	ErrContainerNotFound  = "container-not-found"
	ErrRuntimeUnavailable = "runtime-unavailable"
	ErrPathNotFound       = "path-not-found"
	ErrPermissionDenied   = "permission-denied"
	ErrIO                 = "io-error"
	ErrExpired            = "expired"
	ErrCancelled          = "cancelled"
	ErrDuplicate          = "duplicate"
	ErrRateLimited        = "rate-limited"
	ErrShuttingDown       = "computer-shutting-down"
	ErrInternal           = "internal"
)

const MaxOutputBytes = 100 * 1024

type SecretInjection struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Mode  string `json:"mode"` // "env" | "file"
}

type Envelope struct {
	ID         string            `json:"id"`
	IssuedAt   int64             `json:"issuedAt"`
	TTLMs      int               `json:"ttlMs"`
	Kind       string            `json:"kind"`
	Payload    json.RawMessage   `json:"payload"`
	RunAs      string            `json:"runAs"`
	CWD        string            `json:"cwd,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	Secrets    []SecretInjection `json:"secrets,omitempty"`
	Container  string            `json:"container,omitempty"`
	Priority   string            `json:"priority,omitempty"`
	SelfIssued bool              `json:"selfIssued,omitempty"`
}

type Result struct {
	ID         string          `json:"id"`
	OK         bool            `json:"ok"`
	DurationMs int64           `json:"durationMs"`
	Data       json.RawMessage `json:"data,omitempty"`
	Error      *ResultError    `json:"error,omitempty"`
	Truncated  bool            `json:"truncated,omitempty"`
	Inflight   int             `json:"inflight,omitempty"`
	Queued     int             `json:"queued,omitempty"`
}

type ResultError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Chunk struct {
	ID      string `json:"id"`
	Offset  int64  `json:"offset"`
	Channel string `json:"channel"`
	DataB64 string `json:"dataB64"`
}

type ExecPayload struct {
	Cmd             string `json:"cmd"`
	ShellMode       string `json:"shellMode,omitempty"`
	TimeoutMs       int    `json:"timeoutMs,omitempty"`
	ChunkIntervalMs int    `json:"chunkIntervalMs,omitempty"`
}

type ExecResult struct {
	ExitCode *int   `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	TimedOut bool   `json:"timedOut"`
}

type FileReadPayload struct {
	Path    string `json:"path"`
	Offset  int    `json:"offset,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	FromEnd bool   `json:"fromEnd,omitempty"`
}

type FileWritePayload struct {
	Path       string `json:"path"`
	ContentB64 string `json:"contentB64"`
	Mode       int    `json:"mode,omitempty"`
}

type FileDeletePayload struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive,omitempty"`
}

type FilePathPayload struct {
	Path string `json:"path"`
}

type FileMkdirPayload struct {
	Path    string `json:"path"`
	Mode    int    `json:"mode,omitempty"`
	Parents bool   `json:"parents,omitempty"`
}

type FileMovePayload struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type FileGrepPayload struct {
	Path      string `json:"path"`
	Pattern   string `json:"pattern"`
	Recursive bool   `json:"recursive,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type FileFindPayload struct {
	Path      string `json:"path"`
	Glob      string `json:"glob"`
	Recursive bool   `json:"recursive,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type TmuxRunPayload struct {
	Window         string `json:"window"`
	Cmd            string `json:"cmd"`
	Capture        bool   `json:"capture,omitempty"`
	CaptureDelayMs int    `json:"captureDelayMs,omitempty"`
}

type TmuxCapturePayload struct {
	Window          string `json:"window"`
	Pane            *int   `json:"pane,omitempty"`
	ScrollbackLines int    `json:"scrollbackLines,omitempty"`
}

type TmuxSendKeysPayload struct {
	Window         string          `json:"window"`
	Pane           *int            `json:"pane,omitempty"`
	Keys           json.RawMessage `json:"keys"` // string OR []string
	DelayMs        int             `json:"delayMs,omitempty"`
	Capture        bool            `json:"capture,omitempty"`
	CaptureDelayMs int             `json:"captureDelayMs,omitempty"`
}

type UserCreatePayload struct {
	Username string   `json:"username"`
	Groups   []string `json:"groups,omitempty"`
	Shell    string   `json:"shell,omitempty"`
}

type UsernamePayload struct {
	Username string `json:"username"`
}

type UserEditGroupsPayload struct {
	Username string   `json:"username"`
	Groups   []string `json:"groups"`
}

type EnvSetPayload struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Value    string `json:"value"`
}

type EnvDeletePayload struct {
	Username string `json:"username"`
	Name     string `json:"name"`
}

type CancelPayload struct {
	TargetCommandID string `json:"targetCommandId"`
}

type DesktopPayload struct {
	Action          string `json:"action"`
	X               *int   `json:"x,omitempty"`
	Y               *int   `json:"y,omitempty"`
	StartX          *int   `json:"startX,omitempty"`
	StartY          *int   `json:"startY,omitempty"`
	Text            string `json:"text,omitempty"`
	ScrollDirection string `json:"scrollDirection,omitempty"`
	ScrollAmount    int    `json:"scrollAmount,omitempty"`
	DurationMs      int    `json:"durationMs,omitempty"`
}

type DesktopScreenshotResult struct {
	ImageB64 string `json:"imageB64"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

type DesktopCursorPositionResult struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type DesktopActionAck struct {
	OK     bool   `json:"ok"`
	Action string `json:"action"`
}

const (
	DesktopActionScreenshot     = "screenshot"
	DesktopActionCursorPosition = "cursor-position"
	DesktopActionMouseMove      = "mouse-move"
	DesktopActionLeftClick      = "left-click"
	DesktopActionRightClick     = "right-click"
	DesktopActionMiddleClick    = "middle-click"
	DesktopActionDoubleClick    = "double-click"
	DesktopActionTripleClick    = "triple-click"
	DesktopActionLeftClickDrag  = "left-click-drag"
	DesktopActionLeftMouseDown  = "left-mouse-down"
	DesktopActionLeftMouseUp    = "left-mouse-up"
	DesktopActionScroll         = "scroll"
	DesktopActionKey            = "key"
	DesktopActionType           = "type"
	DesktopActionHoldKey        = "hold-key"
	DesktopActionWait           = "wait"
)

type LocalInferenceRuntimePayload struct {
	Runtime string `json:"runtime"`
	BaseURL string `json:"baseUrl,omitempty"`
	Managed bool   `json:"managed,omitempty"`
	Version string `json:"version,omitempty"`
}

type ComputerAppRuntimePayload struct {
	Runtime string `json:"runtime"`
	Managed bool   `json:"managed,omitempty"`
}

type ComputerAppBasePayload struct {
	AppResourceID string `json:"appResourceId,omitempty"`
	AppSlug       string `json:"appSlug,omitempty"`
}

type ComputerAppResourceLimits struct {
	CPUs        float64 `json:"cpus,omitempty"`
	MemoryBytes int64   `json:"memoryBytes,omitempty"`
	PidsLimit   int     `json:"pidsLimit,omitempty"`
}

type ComputerAppCreatePayload struct {
	AppResourceID  string                    `json:"appResourceId"`
	Name           string                    `json:"name"`
	Slug           string                    `json:"slug"`
	Image          string                    `json:"image"`
	Command        string                    `json:"command,omitempty"`
	Env            map[string]string         `json:"env,omitempty"`
	Labels         map[string]string         `json:"labels"`
	ResourceLimits ComputerAppResourceLimits `json:"resourceLimits,omitempty"`
}

type ComputerAppRunPayload struct {
	AppResourceID  string                    `json:"appResourceId"`
	Name           string                    `json:"name"`
	Slug           string                    `json:"slug"`
	Path           string                    `json:"path"`
	Dockerfile     string                    `json:"dockerfile,omitempty"`
	Env            map[string]string         `json:"env,omitempty"`
	Labels         map[string]string         `json:"labels"`
	ResourceLimits ComputerAppResourceLimits `json:"resourceLimits,omitempty"`
}

type ComputerAppComposeUpPayload struct {
	AppResourceID        string                    `json:"appResourceId"`
	Name                 string                    `json:"name"`
	Slug                 string                    `json:"slug"`
	File                 string                    `json:"file"`
	ProjectDirectory     string                    `json:"projectDirectory"`
	Env                  map[string]string         `json:"env,omitempty"`
	Labels               map[string]string         `json:"labels"`
	ResourceLimits       ComputerAppResourceLimits `json:"resourceLimits,omitempty"`
	AcceptPolicyWarnings bool                      `json:"acceptPolicyWarnings,omitempty"`
}

type ComputerAppLogsPayload struct {
	ComputerAppBasePayload
	Lines   int    `json:"lines,omitempty"`
	Follow  bool   `json:"follow,omitempty"`
	Service string `json:"service,omitempty"`
}

type ComputerAppExecPayload struct {
	ComputerAppBasePayload
	Cmd       string `json:"cmd"`
	Service   string `json:"service,omitempty"`
	TimeoutMs int    `json:"timeoutMs,omitempty"`
}

type ComputerAppExposePayload struct {
	ComputerAppBasePayload
	Port     int    `json:"port"`
	Protocol string `json:"protocol,omitempty"`
	Public   bool   `json:"public,omitempty"`
	Label    string `json:"label,omitempty"`
}

type LocalInferenceModelPayload struct {
	Runtime string `json:"runtime"`
	BaseURL string `json:"baseUrl,omitempty"`
	Model   string `json:"model"`
}

type LocalInferenceModelCreatePayload struct {
	Runtime     string `json:"runtime"`
	BaseURL     string `json:"baseUrl,omitempty"`
	SourceModel string `json:"sourceModel"`
	TargetModel string `json:"targetModel"`
	NumCtx      int    `json:"numCtx"`
}

type LocalInferenceChatPayload struct {
	Runtime string          `json:"runtime"`
	BaseURL string          `json:"baseUrl"`
	Body    json.RawMessage `json:"body"`
}

const DaemonSelfUser = "_daemon"
