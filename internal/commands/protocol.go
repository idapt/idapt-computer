package commands

import "encoding/json"

const (
	KindExec           = "exec"
	KindExecStream     = "exec-stream"
	KindFileRead       = "file-read"
	KindFileWrite      = "file-write"
	KindFileDelete     = "file-delete"
	KindFileList       = "file-list"
	KindFileStat       = "file-stat"
	KindFileMkdir      = "file-mkdir"
	KindFileMove       = "file-move"
	KindFileGrep       = "file-grep"
	KindFileFind       = "file-find"
	KindFileUpload     = "file-upload"
	KindFileDownload   = "file-download"
	KindTmuxRun        = "tmux-run"
	KindTmuxCapture    = "tmux-capture"
	KindTmuxSendKeys   = "tmux-send-keys"
	KindTmuxList       = "tmux-list"
	KindTmuxKill       = "tmux-kill"
	KindUserList       = "user-list"
	KindUserCreate     = "user-create"
	KindUserDelete     = "user-delete"
	KindUserEditGroups = "user-edit-groups"
	KindEnvList        = "env-list"
	KindEnvSet         = "env-set"
	KindEnvDelete      = "env-delete"
	KindPortDiscover   = "port-discover"
	KindHealth         = "health"
	KindShutdown       = "shutdown"
)

const (
	ErrInvalidPayload     = "invalid-payload"
	ErrUnsupportedKind    = "unsupported-kind"
	ErrRunAsNotFound      = "runas-not-found"
	ErrRunAsForbidden     = "runas-forbidden"
	ErrCommandTimeout     = "command-timeout"
	ErrOutputTruncated    = "output-truncated"
	ErrContainerNotFound  = "container-not-found"
	ErrPathNotFound       = "path-not-found"
	ErrPermissionDenied   = "permission-denied"
	ErrIO                 = "io-error"
	ErrExpired            = "expired"
	ErrDuplicate          = "duplicate"
	ErrRateLimited        = "rate-limited"
	ErrShuttingDown       = "machine-shutting-down"
	ErrInternal           = "internal"
)

const MaxOutputBytes = 100 * 1024

type SecretInjection struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Mode  string `json:"mode"` // "env" | "file"
}

type Envelope struct {
	ID         string             `json:"id"`
	IssuedAt   int64              `json:"issuedAt"`
	TTLMs      int                `json:"ttlMs"`
	Kind       string             `json:"kind"`
	Payload    json.RawMessage    `json:"payload"`
	RunAs      string             `json:"runAs"`
	CWD        string             `json:"cwd,omitempty"`
	Env        map[string]string  `json:"env,omitempty"`
	Secrets    []SecretInjection  `json:"secrets,omitempty"`
	Container  string             `json:"container,omitempty"`
	Priority   string             `json:"priority,omitempty"`
	SelfIssued bool               `json:"selfIssued,omitempty"`
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

type ExecPayload struct {
	Cmd       string `json:"cmd"`
	ShellMode string `json:"shellMode,omitempty"`
	TimeoutMs int    `json:"timeoutMs,omitempty"`
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
	Window          string `json:"window"`
	Cmd             string `json:"cmd"`
	Capture         bool   `json:"capture,omitempty"`
	CaptureDelayMs  int    `json:"captureDelayMs,omitempty"`
}

type TmuxCapturePayload struct {
	Window          string `json:"window"`
	Pane            *int   `json:"pane,omitempty"`
	ScrollbackLines int    `json:"scrollbackLines,omitempty"`
}

type TmuxSendKeysPayload struct {
	Window          string          `json:"window"`
	Pane            *int            `json:"pane,omitempty"`
	Keys            json.RawMessage `json:"keys"` // string OR []string
	DelayMs         int             `json:"delayMs,omitempty"`
	Capture         bool            `json:"capture,omitempty"`
	CaptureDelayMs  int             `json:"captureDelayMs,omitempty"`
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

const DaemonSelfUser = "_daemon"
