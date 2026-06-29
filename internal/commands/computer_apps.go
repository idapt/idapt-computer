package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const idaptDockerNetwork = "idapt-apps"

var appSlugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)

type appRuntimeStatus struct {
	Runtime        string  `json:"runtime"`
	Status         string  `json:"status"`
	Available      bool    `json:"available"`
	Version        *string `json:"version"`
	ComposeVersion *string `json:"composeVersion"`
	Context        *string `json:"context"`
	Rootless       *bool   `json:"rootless"`
	Host           *string `json:"host"`
	Message        *string `json:"message"`
	Remediation    *string `json:"remediation"`
}

type appRuntimeSetupResult struct {
	Runtime string           `json:"runtime"`
	Changed bool             `json:"changed"`
	Status  appRuntimeStatus `json:"status"`
}

type appInspectResult struct {
	AppResourceID  string                `json:"appResourceId"`
	Slug           string                `json:"slug"`
	Status         string                `json:"status"`
	Containers     []appRuntimeContainer `json:"containers"`
	Ports          []appRuntimePort      `json:"ports"`
	Volumes        []appRuntimeVolume    `json:"volumes"`
	PolicyWarnings []appPolicyWarning    `json:"policyWarnings,omitempty"`
}

type appListResult struct {
	Apps []appInspectResult `json:"apps"`
}

type appExternalListResult struct {
	Containers []appExternalContainer `json:"containers"`
}

type appRuntimeContainer struct {
	RuntimeContainerID *string `json:"runtimeContainerId"`
	RuntimeName        string  `json:"runtimeName"`
	ServiceName        *string `json:"serviceName"`
	ImageRef           *string `json:"imageRef"`
	Status             string  `json:"status"`
	Health             *string `json:"health"`
}

type appRuntimePort struct {
	RuntimeContainerID *string `json:"runtimeContainerId,omitempty"`
	ServiceName        *string `json:"serviceName,omitempty"`
	Port               int     `json:"port"`
	Protocol           string  `json:"protocol"`
}

type appRuntimeVolume struct {
	RuntimeName string  `json:"runtimeName"`
	Purpose     *string `json:"purpose"`
	Retained    bool    `json:"retained"`
}

type appPolicyWarning struct {
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Field   *string `json:"field"`
	Service *string `json:"service"`
}

type appExternalContainer struct {
	RuntimeContainerID string           `json:"runtimeContainerId"`
	RuntimeName        string           `json:"runtimeName"`
	ImageRef           *string          `json:"imageRef"`
	Status             string           `json:"status"`
	Health             *string          `json:"health"`
	ComposeProject     *string          `json:"composeProject"`
	ComposeService     *string          `json:"composeService"`
	Ports              []appRuntimePort `json:"ports"`
}

type appLogsResult struct {
	Logs      string `json:"logs"`
	Truncated bool   `json:"truncated"`
}

func runComputerAppRuntimeStatus(ctx context.Context, env *Envelope) Result {
	start := time.Now()
	status := detectDockerRuntime(ctx)
	dataBytes, _ := json.Marshal(status)
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runComputerAppRuntimeSetup(ctx context.Context, env *Envelope) Result {
	start := time.Now()
	status := detectDockerRuntime(ctx)
	if !status.Available {
		return errResult(env.ID, ErrRuntimeUnavailable, derefString(status.Message, "Docker runtime is not available"), start)
	}
	status = detectDockerRuntime(ctx)
	dataBytes, _ := json.Marshal(appRuntimeSetupResult{Runtime: "docker", Changed: false, Status: status})
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runComputerAppList(ctx context.Context, env *Envelope) Result {
	start := time.Now()
	appIDs, err := listAppResourceIDs(ctx)
	if err != nil {
		return errResult(env.ID, ErrRuntimeUnavailable, err.Error(), start)
	}
	apps := make([]appInspectResult, 0, len(appIDs))
	for _, appID := range appIDs {
		inspect, err := inspectApp(ctx, ComputerAppBasePayload{AppResourceID: appID})
		if err == nil {
			apps = append(apps, inspect)
		}
	}
	dataBytes, _ := json.Marshal(appListResult{Apps: apps})
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runComputerAppExternalList(ctx context.Context, env *Envelope) Result {
	start := time.Now()
	containers, err := listExternalContainers(ctx)
	if err != nil {
		return errResult(env.ID, ErrRuntimeUnavailable, err.Error(), start)
	}
	dataBytes, _ := json.Marshal(appExternalListResult{Containers: containers})
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runComputerAppInspect(ctx context.Context, env *Envelope) Result {
	start := time.Now()
	var payload ComputerAppBasePayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	inspect, err := inspectApp(ctx, payload)
	if err != nil {
		return errResult(env.ID, ErrContainerNotFound, err.Error(), start)
	}
	dataBytes, _ := json.Marshal(inspect)
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runComputerAppCreate(ctx context.Context, env *Envelope) Result {
	start := time.Now()
	var payload ComputerAppCreatePayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if err := validateAppIdentity(payload.AppResourceID, payload.Slug); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if err := ensureDockerReady(ctx); err != nil {
		return errResult(env.ID, ErrRuntimeUnavailable, err.Error(), start)
	}
	name := runtimeName(payload.Slug)
	args := []string{"run", "-d", "--name", name, "--publish-all"}
	args = append(args, dockerLabels(payload.Labels)...)
	args = append(args, dockerLimits(payload.ResourceLimits)...)
	args = append(args, "--security-opt", "no-new-privileges:true")
	for key, value := range payload.Env {
		args = append(args, "-e", key+"="+value)
	}
	args = append(args, payload.Image)
	if payload.Command != "" {
		args = append(args, "sh", "-lc", payload.Command)
	}
	if err := docker(ctx, args...); err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	inspect, err := inspectApp(ctx, ComputerAppBasePayload{AppResourceID: payload.AppResourceID, AppSlug: payload.Slug})
	if err != nil {
		return errResult(env.ID, ErrInternal, err.Error(), start)
	}
	dataBytes, _ := json.Marshal(inspect)
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runComputerAppRun(ctx context.Context, env *Envelope) Result {
	start := time.Now()
	var payload ComputerAppRunPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if err := validateAppIdentity(payload.AppResourceID, payload.Slug); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if err := ensureDockerReady(ctx); err != nil {
		return errResult(env.ID, ErrRuntimeUnavailable, err.Error(), start)
	}
	if err := validateProjectPath(payload.Path); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if payload.Dockerfile != "" {
		if err := validateProjectPath(payload.Dockerfile); err != nil {
			return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
		}
	}
	image := "idapt-app-" + payload.Slug + ":latest"
	buildArgs := []string{"build", "-t", image}
	buildArgs = append(buildArgs, dockerLabels(payload.Labels)...)
	if payload.Dockerfile != "" {
		buildArgs = append(buildArgs, "-f", payload.Dockerfile)
	}
	buildArgs = append(buildArgs, payload.Path)
	if err := docker(ctx, buildArgs...); err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	createPayload := ComputerAppCreatePayload{
		AppResourceID:  payload.AppResourceID,
		Name:           payload.Name,
		Slug:           payload.Slug,
		Image:          image,
		Env:            payload.Env,
		Labels:         payload.Labels,
		ResourceLimits: payload.ResourceLimits,
	}
	payloadBytes, _ := json.Marshal(createPayload)
	env.Payload = payloadBytes
	return runComputerAppCreate(ctx, env)
}

func runComputerAppComposeUp(ctx context.Context, env *Envelope) Result {
	start := time.Now()
	var payload ComputerAppComposeUpPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if err := validateAppIdentity(payload.AppResourceID, payload.Slug); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if err := ensureDockerReady(ctx); err != nil {
		return errResult(env.ID, ErrRuntimeUnavailable, err.Error(), start)
	}
	if err := validateProjectPath(payload.ProjectDirectory); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if err := validateProjectPath(payload.File); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	composeFile := filepath.Join(payload.ProjectDirectory, payload.File)
	policyReport, err := validateComposePolicy(composeFile, filepath.Dir(composeFile))
	if err != nil {
		return errResult(env.ID, ErrPermissionDenied, err.Error(), start)
	}
	if len(policyReport.Warnings) > 0 && !payload.AcceptPolicyWarnings {
		return errResult(env.ID, ErrPermissionDenied, formatPolicyWarnings(policyReport.Warnings), start)
	}
	overrideFile, cleanup, err := writeComposeLabelOverride(composeFile, payload.Labels)
	if err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	defer cleanup()
	args := []string{"compose", "-p", runtimeName(payload.Slug), "-f", composeFile, "-f", overrideFile, "up", "-d", "--build"}
	if _, err := dockerWithEnv(ctx, payload.Env, args...); err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	inspect, err := inspectApp(ctx, ComputerAppBasePayload{AppResourceID: payload.AppResourceID, AppSlug: payload.Slug})
	if err != nil {
		return errResult(env.ID, ErrInternal, err.Error(), start)
	}
	inspect.PolicyWarnings = policyReport.Warnings
	dataBytes, _ := json.Marshal(inspect)
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runComputerAppLifecycle(ctx context.Context, env *Envelope, action string) Result {
	start := time.Now()
	var payload ComputerAppBasePayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	ids, err := containerIDsForApp(ctx, payload)
	if err != nil {
		return errResult(env.ID, ErrContainerNotFound, err.Error(), start)
	}
	if len(ids) == 0 {
		return errResult(env.ID, ErrContainerNotFound, "Computer App has no containers", start)
	}
	dockerAction := map[string]string{
		"start":   "start",
		"stop":    "stop",
		"restart": "restart",
		"delete":  "rm",
		"reset":   "rm",
	}[action]
	args := []string{dockerAction}
	if action == "delete" || action == "reset" {
		args = append(args, "-f")
	}
	args = append(args, ids...)
	if err := docker(ctx, args...); err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	if action == "delete" || action == "reset" {
		dataBytes, _ := json.Marshal(appInspectResult{AppResourceID: payload.AppResourceID, Slug: payload.AppSlug, Status: "stopped"})
		return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
	}
	inspect, err := inspectApp(ctx, payload)
	if err != nil {
		return errResult(env.ID, ErrInternal, err.Error(), start)
	}
	dataBytes, _ := json.Marshal(inspect)
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runComputerAppLogs(ctx context.Context, env *Envelope) Result {
	start := time.Now()
	var payload ComputerAppLogsPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	ids, err := containerIDsForAppService(ctx, payload.ComputerAppBasePayload, payload.Service)
	if err != nil || len(ids) == 0 {
		return errResult(env.ID, ErrContainerNotFound, "Computer App has no containers", start)
	}
	lines := payload.Lines
	if lines <= 0 || lines > 5000 {
		lines = 200
	}
	args := []string{"logs", "--tail", strconv.Itoa(lines)}
	args = append(args, ids...)
	out, err := dockerCombinedOutput(ctx, args...)
	if err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	truncated := false
	if len(out) > MaxOutputBytes {
		out = out[:MaxOutputBytes]
		truncated = true
	}
	dataBytes, _ := json.Marshal(appLogsResult{Logs: out, Truncated: truncated})
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes, Truncated: truncated}
}

func runComputerAppExec(ctx context.Context, env *Envelope) Result {
	start := time.Now()
	var payload ComputerAppExecPayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if payload.Cmd == "" {
		return errResult(env.ID, ErrInvalidPayload, "cmd required", start)
	}
	ids, err := containerIDsForAppService(ctx, payload.ComputerAppBasePayload, payload.Service)
	if err != nil || len(ids) == 0 {
		return errResult(env.ID, ErrContainerNotFound, "Computer App has no containers", start)
	}
	timeout := SafeTimeout(env.TTLMs)
	if payload.TimeoutMs > 0 {
		timeout = time.Duration(payload.TimeoutMs) * time.Millisecond
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, "docker", "exec", ids[0], "sh", "-lc", payload.Cmd)
	stdoutBuf := newCapped(MaxOutputBytes)
	stderrBuf := newCapped(MaxOutputBytes)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf
	err = cmd.Run()
	exitCode := -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	} else if err == nil {
		exitCode = 0
	}
	timedOut := cctx.Err() == context.DeadlineExceeded
	dataBytes, _ := json.Marshal(ExecResult{
		ExitCode: ptrInt(exitCode),
		Stdout:   string(stdoutBuf.Bytes()),
		Stderr:   string(stderrBuf.Bytes()),
		TimedOut: timedOut,
	})
	res := Result{ID: env.ID, OK: !timedOut && exitCode != -1, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes, Truncated: stdoutBuf.Truncated || stderrBuf.Truncated}
	if timedOut {
		res.Error = &ResultError{Code: ErrCommandTimeout, Message: "command exceeded timeout"}
	}
	return res
}

func runComputerAppExpose(ctx context.Context, env *Envelope, expose bool) Result {
	start := time.Now()
	var payload ComputerAppExposePayload
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if payload.Port <= 0 || payload.Port > 65535 {
		return errResult(env.ID, ErrInvalidPayload, "port must be 1-65535", start)
	}
	exe, err := os.Executable()
	if err != nil {
		return errResult(env.ID, ErrInternal, err.Error(), start)
	}
	args := []string{"unexpose", strconv.Itoa(payload.Port)}
	if expose {
		authMode := "private"
		if payload.Public {
			authMode = "idapt"
		}
		args = []string{"expose", strconv.Itoa(payload.Port), "--auth", authMode}
	}
	out, err := exec.CommandContext(ctx, exe, args...).CombinedOutput()
	if err != nil {
		return errResult(env.ID, ErrIO, strings.TrimSpace(string(out)), start)
	}
	inspect, err := inspectApp(ctx, payload.ComputerAppBasePayload)
	if err != nil {
		return errResult(env.ID, ErrContainerNotFound, err.Error(), start)
	}
	found := false
	for _, port := range inspect.Ports {
		if port.Port == payload.Port && (payload.Protocol == "" || port.Protocol == payload.Protocol) {
			found = true
			break
		}
	}
	if expose && !found {
		inspect.Ports = append(inspect.Ports, appRuntimePort{Port: payload.Port, Protocol: firstNonEmpty(payload.Protocol, "tcp")})
	}
	dataBytes, _ := json.Marshal(inspect)
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func detectDockerRuntime(ctx context.Context) appRuntimeStatus {
	if _, err := exec.LookPath("docker"); err != nil {
		msg := "Docker CLI is not installed"
		remediation := "Install Docker Desktop, Docker Engine, or a compatible Docker CLI runtime, then retry."
		return appRuntimeStatus{Runtime: "docker", Status: "not-installed", Available: false, Message: &msg, Remediation: &remediation}
	}
	version, err := dockerOutput(ctx, "version", "--format", "{{.Server.Version}}")
	if err != nil {
		msg := "Docker daemon is not reachable"
		remediation := "Start Docker Desktop or the Docker daemon, then retry."
		return appRuntimeStatus{Runtime: "docker", Status: "unhealthy", Available: false, Message: &msg, Remediation: &remediation}
	}
	composeVersion, _ := dockerOutput(ctx, "compose", "version", "--short")
	contextName, _ := dockerOutput(ctx, "context", "show")
	info, _ := dockerOutput(ctx, "info", "--format", "{{json .SecurityOptions}}|{{.OperatingSystem}}")
	rootless := strings.Contains(info, "rootless")
	host := strings.TrimSpace(info)
	return appRuntimeStatus{
		Runtime:        "docker",
		Status:         "running",
		Available:      true,
		Version:        ptrString(strings.TrimSpace(version)),
		ComposeVersion: ptrStringOrNil(strings.TrimSpace(composeVersion)),
		Context:        ptrStringOrNil(strings.TrimSpace(contextName)),
		Rootless:       &rootless,
		Host:           ptrStringOrNil(host),
	}
}

func ensureDockerReady(ctx context.Context) error {
	status := detectDockerRuntime(ctx)
	if !status.Available {
		return fmt.Errorf("%s", derefString(status.Message, "Docker runtime is unavailable"))
	}
	return nil
}

func ensureNetwork(ctx context.Context) error {
	if err := docker(ctx, "network", "inspect", idaptDockerNetwork); err == nil {
		return nil
	}
	return docker(ctx, "network", "create", "--driver", "bridge", "--label", "ai.idapt.managed=true", idaptDockerNetwork)
}

func inspectApp(ctx context.Context, payload ComputerAppBasePayload) (appInspectResult, error) {
	ids, err := containerIDsForApp(ctx, payload)
	if err != nil {
		return appInspectResult{}, err
	}
	if len(ids) == 0 {
		return appInspectResult{}, errors.New("Computer App container not found")
	}
	raw, err := dockerOutput(ctx, append([]string{"inspect"}, ids...)...)
	if err != nil {
		return appInspectResult{}, err
	}
	var docs []dockerInspect
	if err := json.Unmarshal([]byte(raw), &docs); err != nil {
		return appInspectResult{}, err
	}
	result := appInspectResult{AppResourceID: payload.AppResourceID, Slug: payload.AppSlug, Status: "stopped"}
	statusRank := 0
	for _, doc := range docs {
		labels := doc.Config.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		appID := labels["ai.idapt.app_resource_id"]
		if result.AppResourceID == "" {
			result.AppResourceID = appID
		}
		if result.Slug == "" {
			result.Slug = strings.TrimPrefix(doc.Name, "/idapt-")
		}
		runtimeID := doc.ID
		service := nullableString(labels["com.docker.compose.service"])
		image := nullableString(doc.Config.Image)
		health := nullableString("")
		if doc.State.Health != nil {
			health = nullableString(doc.State.Health.Status)
		}
		result.Containers = append(result.Containers, appRuntimeContainer{
			RuntimeContainerID: &runtimeID,
			RuntimeName:        strings.TrimPrefix(doc.Name, "/"),
			ServiceName:        service,
			ImageRef:           image,
			Status:             doc.State.Status,
			Health:             health,
		})
		result.Ports = append(result.Ports, portsFromInspect(doc, &runtimeID, service)...)
		for _, mount := range doc.Mounts {
			if mount.Type == "volume" && mount.Name != "" {
				result.Volumes = append(result.Volumes, appRuntimeVolume{RuntimeName: mount.Name, Retained: true})
			}
		}
		rank := appStatusRank(doc.State.Status, health)
		if rank > statusRank {
			statusRank = rank
			result.Status = appStatusFromRank(rank)
		}
	}
	result.Ports = dedupePorts(result.Ports)
	sort.Slice(result.Containers, func(i, j int) bool { return result.Containers[i].RuntimeName < result.Containers[j].RuntimeName })
	sort.Slice(result.Ports, func(i, j int) bool { return result.Ports[i].Port < result.Ports[j].Port })
	return result, nil
}

func listExternalContainers(ctx context.Context) ([]appExternalContainer, error) {
	rawIDs, err := dockerOutput(ctx, "ps", "-a", "--format", "{{.ID}}")
	if err != nil {
		return nil, err
	}
	ids := strings.Fields(rawIDs)
	if len(ids) == 0 {
		return []appExternalContainer{}, nil
	}
	raw, err := dockerOutput(ctx, append([]string{"inspect"}, ids...)...)
	if err != nil {
		return nil, err
	}
	var docs []dockerInspect
	if err := json.Unmarshal([]byte(raw), &docs); err != nil {
		return nil, err
	}
	containers := []appExternalContainer{}
	for _, doc := range docs {
		labels := doc.Config.Labels
		if labels == nil {
			labels = map[string]string{}
		}
		if labels["ai.idapt.managed"] == "true" {
			continue
		}
		runtimeID := doc.ID
		service := nullableString(labels["com.docker.compose.service"])
		container := appExternalContainer{
			RuntimeContainerID: runtimeID,
			RuntimeName:        strings.TrimPrefix(doc.Name, "/"),
			ImageRef:           nullableString(doc.Config.Image),
			Status:             doc.State.Status,
			Health:             nil,
			ComposeProject:     nullableString(labels["com.docker.compose.project"]),
			ComposeService:     service,
			Ports:              portsFromInspect(doc, &runtimeID, service),
		}
		if doc.State.Health != nil {
			container.Health = nullableString(doc.State.Health.Status)
		}
		container.Ports = dedupePorts(container.Ports)
		containers = append(containers, container)
	}
	sort.Slice(containers, func(i, j int) bool { return containers[i].RuntimeName < containers[j].RuntimeName })
	return containers, nil
}

func portsFromInspect(doc dockerInspect, runtimeID *string, service *string) []appRuntimePort {
	ports := []appRuntimePort{}
	for key, bindings := range doc.NetworkSettings.Ports {
		containerPort, proto := parseDockerPortKey(key)
		if len(bindings) == 0 && containerPort > 0 {
			ports = append(ports, appRuntimePort{RuntimeContainerID: runtimeID, ServiceName: service, Port: containerPort, Protocol: proto})
			continue
		}
		for _, binding := range bindings {
			hostPort, err := strconv.Atoi(binding.HostPort)
			if err == nil && hostPort > 0 {
				ports = append(ports, appRuntimePort{RuntimeContainerID: runtimeID, ServiceName: service, Port: hostPort, Protocol: proto})
			}
		}
	}
	for key := range doc.Config.ExposedPorts {
		if _, hasMapping := doc.NetworkSettings.Ports[key]; hasMapping {
			continue
		}
		port, proto := parseDockerPortKey(key)
		if port > 0 {
			ports = append(ports, appRuntimePort{RuntimeContainerID: runtimeID, ServiceName: service, Port: port, Protocol: proto})
		}
	}
	return ports
}

type dockerInspect struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Config struct {
		Image        string            `json:"Image"`
		Labels       map[string]string `json:"Labels"`
		ExposedPorts map[string]any    `json:"ExposedPorts"`
	} `json:"Config"`
	State struct {
		Status string `json:"Status"`
		Health *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	NetworkSettings struct {
		Ports map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
	} `json:"NetworkSettings"`
	Mounts []struct {
		Type string `json:"Type"`
		Name string `json:"Name"`
	} `json:"Mounts"`
}

func listAppResourceIDs(ctx context.Context) ([]string, error) {
	raw, err := dockerOutput(ctx, "ps", "-a", "--filter", "label=ai.idapt.managed=true", "--format", "{{.Label \"ai.idapt.app_resource_id\"}}")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(raw, "\n") {
		id := strings.TrimSpace(line)
		if id != "" && !strings.HasPrefix(id, "<no") {
			seen[id] = true
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

func containerIDsForApp(ctx context.Context, payload ComputerAppBasePayload) ([]string, error) {
	if payload.AppResourceID != "" {
		ids, err := containerIDsByFilter(ctx, "label=ai.idapt.app_resource_id="+payload.AppResourceID)
		if err != nil {
			return nil, err
		}
		if len(ids) > 0 || payload.AppSlug == "" {
			return ids, nil
		}
	}
	if payload.AppSlug != "" {
		return containerIDsByFilter(ctx, "name="+runtimeName(payload.AppSlug))
	}
	return nil, errors.New("appResourceId or appSlug required")
}

func containerIDsForAppService(ctx context.Context, payload ComputerAppBasePayload, service string) ([]string, error) {
	ids, err := containerIDsForApp(ctx, payload)
	if err != nil || service == "" {
		return ids, err
	}
	raw, err := dockerOutput(ctx, append([]string{"inspect"}, ids...)...)
	if err != nil {
		return nil, err
	}
	var docs []dockerInspect
	if err := json.Unmarshal([]byte(raw), &docs); err != nil {
		return nil, err
	}
	filtered := []string{}
	for _, doc := range docs {
		if doc.Config.Labels["com.docker.compose.service"] == service {
			filtered = append(filtered, doc.ID)
		}
	}
	return filtered, nil
}

func containerIDsByFilter(ctx context.Context, filter string) ([]string, error) {
	raw, err := dockerOutput(ctx, "ps", "-a", "--filter", filter, "--format", "{{.ID}}")
	if err != nil {
		return nil, err
	}
	return strings.Fields(raw), nil
}

func validateAppIdentity(appResourceID, slug string) error {
	if appResourceID == "" {
		return errors.New("appResourceId required")
	}
	if !appSlugRegex.MatchString(slug) {
		return errors.New("slug must be lowercase letters, numbers, and hyphens")
	}
	return nil
}

func validateProjectPath(path string) error {
	if path == "" || strings.Contains(path, "\x00") {
		return errors.New("path is invalid")
	}
	clean := filepath.Clean(path)
	if filepath.IsAbs(clean) {
		return errors.New("absolute host paths are not allowed")
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return errors.New("paths outside the project are not allowed")
	}
	return nil
}

type composePolicyReport struct {
	Warnings []appPolicyWarning
}

func validateComposePolicy(path string, projectDir string) (composePolicyReport, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return composePolicyReport{}, err
	}
	var root map[string]any
	if err := yaml.Unmarshal(content, &root); err != nil {
		return composePolicyReport{}, err
	}
	report := composePolicyReport{}
	services, _ := root["services"].(map[string]any)
	for serviceName, rawService := range services {
		service, _ := rawService.(map[string]any)
		if boolValue(service["privileged"]) {
			return report, fmt.Errorf("service %s uses privileged mode, which is blocked", serviceName)
		}
		for _, field := range []string{"network_mode", "pid", "ipc", "uts"} {
			if strings.EqualFold(stringValue(service[field]), "host") {
				return report, fmt.Errorf("service %s uses %s=host, which is blocked", serviceName, field)
			}
		}
		if _, ok := service["devices"]; ok {
			return report, fmt.Errorf("service %s declares devices, which are blocked", serviceName)
		}
		if _, ok := service["cap_add"]; ok {
			return report, fmt.Errorf("service %s adds Linux capabilities, which is blocked", serviceName)
		}
		if warnings, err := validateComposeVolumes(serviceName, service["volumes"], projectDir); err != nil {
			return report, err
		} else {
			report.Warnings = append(report.Warnings, warnings...)
		}
		if _, ok := service["ports"]; ok {
			service := serviceName
			field := "services." + serviceName + ".ports"
			report.Warnings = append(report.Warnings, appPolicyWarning{
				Code:    "published-port",
				Message: "Compose publishes host ports. Idapt will not create a public URL unless you explicitly expose one, but the port may still bind locally according to Docker.",
				Field:   &field,
				Service: &service,
			})
		}
	}
	return report, nil
}

func writeComposeLabelOverride(composeFile string, labels map[string]string) (string, func(), error) {
	content, err := os.ReadFile(composeFile)
	if err != nil {
		return "", func() {}, err
	}
	var root map[string]any
	if err := yaml.Unmarshal(content, &root); err != nil {
		return "", func() {}, err
	}
	services, ok := root["services"].(map[string]any)
	if !ok || len(services) == 0 {
		return "", func() {}, errors.New("compose file has no services")
	}
	overrideServices := map[string]any{}
	for serviceName := range services {
		serviceLabels := map[string]string{}
		for key, value := range labels {
			serviceLabels[key] = value
		}
		overrideServices[serviceName] = map[string]any{
			"labels": serviceLabels,
		}
	}
	override := map[string]any{"services": overrideServices}
	data, err := yaml.Marshal(override)
	if err != nil {
		return "", func() {}, err
	}
	dir, err := os.MkdirTemp("", "idapt-compose-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, "idapt-labels.compose.yaml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return path, cleanup, nil
}

func validateComposeVolumes(serviceName string, raw any, projectDir string) ([]appPolicyWarning, error) {
	volumes, ok := raw.([]any)
	if !ok {
		return nil, nil
	}
	warnings := []appPolicyWarning{}
	for _, rawVolume := range volumes {
		switch volume := rawVolume.(type) {
		case string:
			source := strings.Split(volume, ":")[0]
			if err := checkMountSource(serviceName, source, projectDir); err != nil {
				return warnings, err
			}
			if isProjectRootWritableBind(volume) {
				warnings = append(warnings, projectRootWritableBindWarning(serviceName))
			}
		case map[string]any:
			source := stringValue(volume["source"])
			typ := stringValue(volume["type"])
			if typ == "bind" {
				if err := checkMountSource(serviceName, source, projectDir); err != nil {
					return warnings, err
				}
			}
			if typ == "bind" && isProjectRootSource(source) && !boolValue(volume["read_only"]) {
				warnings = append(warnings, projectRootWritableBindWarning(serviceName))
			}
		}
	}
	return warnings, nil
}

func checkMountSource(serviceName, source, projectDir string) error {
	if source == "" {
		return nil
	}
	if strings.ContainsAny(source, "$") {
		return fmt.Errorf("service %s bind-mount source %q uses a shell variable, which is not allowed", serviceName, source)
	}
	if isNamedVolume(source) {
		return nil
	}
	if strings.HasPrefix(source, "~") ||
		strings.Contains(source, ".ssh") || strings.Contains(source, ".aws") ||
		strings.Contains(source, ".config/gcloud") || source == "/var/run/docker.sock" {
		return fmt.Errorf("service %s mounts blocked host path %s", serviceName, source)
	}

	abs := source
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(projectDir, source)
	}
	abs = filepath.Clean(abs)
	if resolved, err := resolvePathForPolicy(abs); err == nil {
		abs = resolved
	}

	cleanRoot := filepath.Clean(projectDir)
	if resolvedRoot, err := resolvePathForPolicy(cleanRoot); err == nil {
		cleanRoot = resolvedRoot
	}
	if !pathInside(cleanRoot, abs) {
		return fmt.Errorf("service %s bind-mount source %q escapes the project directory", serviceName, source)
	}
	return nil
}

func isNamedVolume(source string) bool {
	if source == "" {
		return false
	}
	if strings.ContainsRune(source, '/') || strings.HasPrefix(source, ".") || strings.HasPrefix(source, "~") {
		return false
	}
	r := rune(source[0])
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func isProjectRootWritableBind(volume string) bool {
	parts := strings.Split(volume, ":")
	if len(parts) == 0 || !isProjectRootSource(parts[0]) {
		return false
	}
	for _, option := range parts[2:] {
		if option == "ro" || strings.HasPrefix(option, "ro,") || strings.Contains(option, ",ro") {
			return false
		}
	}
	return true
}

func isProjectRootSource(source string) bool {
	clean := filepath.Clean(source)
	return clean == "." || clean == "./"
}

func projectRootWritableBindWarning(serviceName string) appPolicyWarning {
	service := serviceName
	field := "services." + serviceName + ".volumes"
	return appPolicyWarning{
		Code:    "project-root-writable-bind",
		Message: "Compose bind-mounts the project root with write access. Code inside the container can modify files in that project.",
		Field:   &field,
		Service: &service,
	}
}

func formatPolicyWarnings(warnings []appPolicyWarning) string {
	if len(warnings) == 0 {
		return "Compose policy warning must be accepted before running"
	}
	parts := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		parts = append(parts, warning.Code+": "+warning.Message)
	}
	return "Compose has risky settings that require explicit acceptance: " + strings.Join(parts, "; ")
}

func dockerLabels(labels map[string]string) []string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	args := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		args = append(args, "--label", key+"="+labels[key])
	}
	return args
}

func dockerLimits(limits ComputerAppResourceLimits) []string {
	args := []string{}
	if limits.CPUs > 0 {
		args = append(args, "--cpus", strconv.FormatFloat(limits.CPUs, 'f', -1, 64))
	}
	if limits.MemoryBytes > 0 {
		args = append(args, "--memory", strconv.FormatInt(limits.MemoryBytes, 10))
	}
	if limits.PidsLimit > 0 {
		args = append(args, "--pids-limit", strconv.Itoa(limits.PidsLimit))
	}
	return args
}

func runtimeName(slug string) string {
	return "idapt-" + slug
}

func docker(ctx context.Context, args ...string) error {
	_, err := dockerCombinedOutput(ctx, args...)
	return err
}

func dockerOutput(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker %s failed: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

func dockerCombinedOutput(ctx context.Context, args ...string) (string, error) {
	return dockerWithEnv(ctx, nil, args...)
}

func dockerWithEnv(ctx context.Context, env map[string]string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = mergeEnv(os.Environ(), env)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker %s failed: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func parseDockerPortKey(key string) (int, string) {
	parts := strings.Split(key, "/")
	if len(parts) == 0 {
		return 0, "tcp"
	}
	port, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "tcp"
	}
	proto := "tcp"
	if len(parts) > 1 && parts[1] != "" {
		proto = parts[1]
	}
	return port, proto
}

func dedupePorts(ports []appRuntimePort) []appRuntimePort {
	seen := map[string]bool{}
	out := make([]appRuntimePort, 0, len(ports))
	for _, port := range ports {
		key := fmt.Sprintf("%s:%d:%s", derefString(port.ServiceName, ""), port.Port, port.Protocol)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, port)
	}
	return out
}

func appStatusRank(status string, health *string) int {
	if health != nil && *health == "unhealthy" {
		return 3
	}
	switch status {
	case "running":
		return 2
	case "exited", "created":
		return 1
	default:
		return 3
	}
}

func appStatusFromRank(rank int) string {
	switch rank {
	case 2:
		return "running"
	case 1:
		return "stopped"
	default:
		return "unhealthy"
	}
}

func boolValue(v any) bool {
	b, _ := v.(bool)
	return b
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func nullableString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func ptrString(v string) *string { return &v }

func ptrStringOrNil(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func derefString(v *string, fallback string) string {
	if v == nil || *v == "" {
		return fallback
	}
	return *v
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
