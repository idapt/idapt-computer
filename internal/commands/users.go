package commands

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func runUserList(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	if runtime.GOOS != "linux" {
		dataBytes, _ := json.Marshal(map[string]any{"users": []any{}})
		return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
	}

	passwd, err := exec.CommandContext(ctx, "getent", "passwd").Output()
	if err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	type unixUser struct {
		Username string   `json:"username"`
		UID      int      `json:"uid"`
		GID      int      `json:"gid"`
		Groups   []string `json:"groups"`
		Shell    string   `json:"shell"`
		HomeDir  string   `json:"homeDir"`
	}
	users := []unixUser{}
	for _, line := range strings.Split(strings.TrimRight(string(passwd), "\n"), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) < 7 {
			continue
		}
		uid, _ := strconv.Atoi(parts[2])
		if uid > 0 && uid < 1000 && parts[0] != "root" {
			continue
		}
		gid, _ := strconv.Atoi(parts[3])
		groups := []string{}
		if u, err := user.Lookup(parts[0]); err == nil {
			gids, err := u.GroupIds()
			if err == nil {
				for _, g := range gids {
					if grp, err := user.LookupGroupId(g); err == nil {
						groups = append(groups, grp.Name)
					}
				}
			}
		}
		users = append(users, unixUser{
			Username: parts[0],
			UID:      uid,
			GID:      gid,
			Groups:   groups,
			Shell:    parts[6],
			HomeDir:  parts[5],
		})
	}
	dataBytes, _ := json.Marshal(map[string]any{"users": users})
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: dataBytes}
}

func runUserCreate(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	if runtime.GOOS != "linux" {
		return errResult(env.ID, ErrUnsupportedKind, "user-create not supported on this OS", start)
	}
	var p UserCreatePayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if !posixUserRegex.MatchString(p.Username) {
		return errResult(env.ID, ErrInvalidPayload, "malformed username", start)
	}
	if p.Username == "root" {
		return errResult(env.ID, ErrInvalidPayload, "refusing to create reserved user", start)
	}
	if err := validatePrivilegedGroups(p.Groups); err != nil {
		return errResult(env.ID, ErrPermissionDenied, err.Error(), start)
	}
	if err := validateLoginShell(p.Shell); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	args := []string{"-m"}
	if p.Shell != "" {
		args = append(args, "-s", p.Shell)
	}
	if len(p.Groups) > 0 {
		args = append(args, "-G", strings.Join(p.Groups, ","))
	}
	args = append(args, p.Username)
	if err := exec.CommandContext(ctx, "useradd", args...).Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 9 {
			return errResult(env.ID, ErrInvalidPayload, "user already exists", start)
		}
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: json.RawMessage("{}")}
}

func runUserDelete(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	if runtime.GOOS != "linux" {
		return errResult(env.ID, ErrUnsupportedKind, "user-delete not supported on this OS", start)
	}
	var p UsernamePayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if !posixUserRegex.MatchString(p.Username) {
		return errResult(env.ID, ErrInvalidPayload, "malformed username", start)
	}
	if err := exec.CommandContext(ctx, "userdel", "-r", p.Username).Run(); err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: json.RawMessage("{}")}
}

func runUserEditGroups(ctx context.Context, env *Envelope, cfg RunuserConfig) Result {
	start := time.Now()
	if err := ValidateRunAs(env.RunAs, cfg); err != nil {
		return errResult(env.ID, ErrRunAsForbidden, err.Error(), start)
	}
	if runtime.GOOS != "linux" {
		return errResult(env.ID, ErrUnsupportedKind, "user-edit-groups not supported on this OS", start)
	}
	var p UserEditGroupsPayload
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return errResult(env.ID, ErrInvalidPayload, err.Error(), start)
	}
	if !posixUserRegex.MatchString(p.Username) {
		return errResult(env.ID, ErrInvalidPayload, "malformed username", start)
	}
	if p.Username == "root" {
		return errResult(env.ID, ErrInvalidPayload, "refusing to edit root groups", start)
	}
	if err := validatePrivilegedGroups(p.Groups); err != nil {
		return errResult(env.ID, ErrPermissionDenied, err.Error(), start)
	}
	if err := exec.CommandContext(ctx, "usermod", "-G", strings.Join(p.Groups, ","), p.Username).Run(); err != nil {
		return errResult(env.ID, ErrIO, err.Error(), start)
	}
	return Result{ID: env.ID, OK: true, DurationMs: time.Since(start).Milliseconds(), Data: json.RawMessage("{}")}
}
