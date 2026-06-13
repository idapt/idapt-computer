//go:build linux

package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const confineFlags = unix.RESOLVE_IN_ROOT | unix.RESOLVE_NO_SYMLINKS

var errConfinedEscape = errors.New("path escapes the allowed root")

func relUnderHome(homeDir, target string) (string, error) {
	clean := filepath.Clean(target)
	rel, err := filepath.Rel(homeDir, clean)
	if err != nil {
		return "", errConfinedEscape
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errConfinedEscape
	}
	if rel == "." {
		return ".", nil
	}
	return rel, nil
}

func openHomeRoot(owner *runAsOwner) (int, error) {
	fd, err := unix.Open(owner.HomeDir, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("open home root: %w", err)
	}
	return fd, nil
}

func openConfined(owner *runAsOwner, target string, flags int, mode os.FileMode) (*os.File, error) {
	rel, err := relUnderHome(owner.HomeDir, target)
	if err != nil {
		return nil, err
	}
	rootFd, err := openHomeRoot(owner)
	if err != nil {
		return nil, err
	}
	defer unix.Close(rootFd)

	how := &unix.OpenHow{
		Flags:   uint64(flags) | unix.O_CLOEXEC,
		Mode:    uint64(mode),
		Resolve: confineFlags,
	}
	fd, err := unix.Openat2(rootFd, rel, how)
	if err != nil {
		return nil, mapConfineErr(err)
	}
	return os.NewFile(uintptr(fd), filepath.Join(owner.HomeDir, rel)), nil
}

func openParentConfined(owner *runAsOwner, target string) (parentFd int, leaf string, err error) {
	clean := filepath.Clean(target)
	if samePath(clean, owner.HomeDir) {
		return -1, "", fmt.Errorf("refusing to operate on the home root itself")
	}
	parent := filepath.Dir(clean)
	leaf = filepath.Base(clean)
	if leaf == "." || leaf == string(filepath.Separator) || strings.ContainsRune(leaf, filepath.Separator) {
		return -1, "", errConfinedEscape
	}

	rootFd, err := openHomeRoot(owner)
	if err != nil {
		return -1, "", err
	}
	defer unix.Close(rootFd)

	if samePath(parent, owner.HomeDir) {
		dup, derr := unix.Open(owner.HomeDir, unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
		if derr != nil {
			return -1, "", fmt.Errorf("open home root: %w", derr)
		}
		return dup, leaf, nil
	}

	relParent, rerr := relUnderHome(owner.HomeDir, parent)
	if rerr != nil {
		return -1, "", rerr
	}
	how := &unix.OpenHow{
		Flags:   uint64(unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC),
		Resolve: confineFlags,
	}
	fd, oerr := unix.Openat2(rootFd, relParent, how)
	if oerr != nil {
		return -1, "", mapConfineErr(oerr)
	}
	return fd, leaf, nil
}

func chownConfinedNoFollow(owner *runAsOwner, target string) error {
	if os.Geteuid() != 0 {
		return nil
	}
	parentFd, leaf, err := openParentConfined(owner, target)
	if err != nil {
		return err
	}
	defer unix.Close(parentFd)
	if err := unix.Fchownat(parentFd, leaf, owner.UID, owner.GID, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return mapConfineErr(err)
	}
	return nil
}

func mkdirConfined(owner *runAsOwner, target string, mode os.FileMode) error {
	parentFd, leaf, err := openParentConfined(owner, target)
	if err != nil {
		return err
	}
	defer unix.Close(parentFd)
	if err := unix.Mkdirat(parentFd, leaf, uint32(mode.Perm())); err != nil {
		return mapConfineErr(err)
	}
	return nil
}

func mkdirAllConfined(owner *runAsOwner, target string, mode os.FileMode) error {
	rel, err := relUnderHome(owner.HomeDir, target)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil // home already exists
	}
	curFd, err := openHomeRoot(owner)
	if err != nil {
		return err
	}
	defer func() { unix.Close(curFd) }()

	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if merr := unix.Mkdirat(curFd, part, uint32(mode.Perm())); merr != nil && !errors.Is(merr, unix.EEXIST) {
			return mapConfineErr(merr)
		} else if merr == nil && os.Geteuid() == 0 {
			if cfd, oerr := unix.Openat(curFd, part, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0); oerr == nil {
				_ = unix.Fchownat(cfd, "", owner.UID, owner.GID, unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW)
				unix.Close(cfd)
			}
		}
		childFd, oerr := unix.Openat(curFd, part, unix.O_PATH|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if oerr != nil {
			unix.Close(curFd)
			return mapConfineErr(oerr)
		}
		unix.Close(curFd)
		curFd = childFd
	}
	return nil
}

func removeConfined(owner *runAsOwner, target string, recursive bool) error {
	parentFd, leaf, err := openParentConfined(owner, target)
	if err != nil {
		return err
	}
	defer unix.Close(parentFd)

	if !recursive {
		if err := unix.Unlinkat(parentFd, leaf, 0); err != nil {
			if errors.Is(err, unix.EISDIR) {
				if derr := unix.Unlinkat(parentFd, leaf, unix.AT_REMOVEDIR); derr != nil {
					return mapConfineErr(derr)
				}
				return nil
			}
			return mapConfineErr(err)
		}
		return nil
	}
	return removeAllAt(parentFd, leaf)
}

func removeAllAt(dirFd int, name string) error {
	if err := unix.Unlinkat(dirFd, name, 0); err == nil {
		return nil
	} else if !errors.Is(err, unix.EISDIR) && !errors.Is(err, unix.EPERM) {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return mapConfineErr(err)
	}

	childFd, err := unix.Openat(dirFd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.ENOTDIR) || errors.Is(err, unix.ELOOP) {
			if uerr := unix.Unlinkat(dirFd, name, 0); uerr != nil && !errors.Is(uerr, unix.ENOENT) {
				return mapConfineErr(uerr)
			}
			return nil
		}
		return mapConfineErr(err)
	}
	f := os.NewFile(uintptr(childFd), name)
	names, rerr := f.Readdirnames(-1)
	if rerr != nil {
		f.Close()
		return rerr
	}
	for _, n := range names {
		if err := removeAllAt(childFd, n); err != nil {
			f.Close()
			return err
		}
	}
	f.Close()
	if err := unix.Unlinkat(dirFd, name, unix.AT_REMOVEDIR); err != nil && !errors.Is(err, unix.ENOENT) {
		return mapConfineErr(err)
	}
	return nil
}

func renameConfined(owner *runAsOwner, from, to string) error {
	fromFd, fromLeaf, err := openParentConfined(owner, from)
	if err != nil {
		return err
	}
	defer unix.Close(fromFd)
	toFd, toLeaf, err := openParentConfined(owner, to)
	if err != nil {
		return err
	}
	defer unix.Close(toFd)
	if err := unix.Renameat(fromFd, fromLeaf, toFd, toLeaf); err != nil {
		return mapConfineErr(err)
	}
	return nil
}

func mapConfineErr(err error) error {
	switch {
	case errors.Is(err, unix.ELOOP), errors.Is(err, unix.EXDEV):
		return fmt.Errorf("%w: %v", errConfinedEscape, err)
	case errors.Is(err, unix.EACCES), errors.Is(err, unix.EPERM):
		return os.ErrPermission
	case errors.Is(err, unix.ENOENT):
		return os.ErrNotExist
	case errors.Is(err, unix.EEXIST):
		return os.ErrExist
	default:
		return err
	}
}
