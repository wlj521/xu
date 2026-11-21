package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/pkg/sftp"
)

type remoteCopyItem struct {
	source string
	target string
	size   int64
	mode   os.FileMode
	dir    bool
}

type remoteCopyPlan struct {
	items []remoteCopyItem
	total int64
}

func buildRemoteCopyPlan(ctx context.Context, source *sftp.Client, sourceRoot, targetRoot string) (remoteCopyPlan, error) {
	if err := ctx.Err(); err != nil {
		return remoteCopyPlan{}, err
	}
	info, err := source.Stat(sourceRoot)
	if err != nil {
		return remoteCopyPlan{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return remoteCopyPlan{}, fmt.Errorf("暂不跟随符号链接: %s", sourceRoot)
	}
	if info.IsDir() {
		plan := remoteCopyPlan{
			items: []remoteCopyItem{{
				source: sourceRoot,
				target: targetRoot,
				mode:   info.Mode(),
				dir:    true,
			}},
		}
		if err := appendRemoteCopyTree(ctx, source, sourceRoot, targetRoot, &plan); err != nil {
			return remoteCopyPlan{}, err
		}
		return plan, nil
	}
	if !info.Mode().IsRegular() {
		return remoteCopyPlan{}, fmt.Errorf("不是普通文件: %s", sourceRoot)
	}
	return remoteCopyPlan{
		items: []remoteCopyItem{{
			source: sourceRoot,
			target: targetRoot,
			size:   info.Size(),
			mode:   info.Mode(),
		}},
		total: info.Size(),
	}, nil
}

func appendRemoteCopyTree(ctx context.Context, source *sftp.Client, sourceDir, targetDir string, plan *remoteCopyPlan) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := source.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			continue
		}
		sourcePath := path.Join(sourceDir, entry.Name())
		targetPath := path.Join(targetDir, entry.Name())
		switch {
		case entry.IsDir():
			plan.items = append(plan.items, remoteCopyItem{
				source: sourcePath,
				target: targetPath,
				mode:   entry.Mode(),
				dir:    true,
			})
			if err := appendRemoteCopyTree(ctx, source, sourcePath, targetPath, plan); err != nil {
				return err
			}
		case entry.Mode().IsRegular():
			plan.items = append(plan.items, remoteCopyItem{
				source: sourcePath,
				target: targetPath,
				size:   entry.Size(),
				mode:   entry.Mode(),
			})
			plan.total += entry.Size()
		}
	}
	return nil
}

// copyRemoteWithState streams one SFTP session into another through memory,
// never staging the complete file on local disk.
func copyRemoteWithState(ctx context.Context, source, target *sftp.Client, sourceRoot, targetDir string, conflict transferConflict, sourceIsDir bool, report func(int64, int64), state *transferResumeState) error {
	if report == nil {
		report = func(int64, int64) {}
	}
	if state == nil {
		state = newTransferResumeState()
	}
	sourceRoot = path.Clean(sourceRoot)
	targetDir = path.Clean(strings.TrimSpace(targetDir))
	if sourceRoot == "." || sourceRoot == "/" || !strings.HasPrefix(sourceRoot, "/") {
		return errors.New("源路径无效")
	}
	if targetDir == "." || !strings.HasPrefix(targetDir, "/") {
		return errors.New("目标目录无效")
	}

	info, err := source.Stat(sourceRoot)
	if err != nil {
		return fmt.Errorf("读取源路径: %w", err)
	}
	if info.IsDir() != sourceIsDir {
		return errors.New("源路径类型已变化，请重新选择")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("暂不跟随符号链接: %s", sourceRoot)
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return fmt.Errorf("不是普通文件: %s", sourceRoot)
	}
	if err := target.MkdirAll(targetDir); err != nil {
		return fmt.Errorf("创建目标目录 %s: %w", targetDir, err)
	}

	defaultRoot := path.Join(targetDir, path.Base(sourceRoot))
	rootKey := "remote-root:" + sourceRoot
	if _, cached := state.targets[rootKey]; !cached {
		if err := validateRemoteCopyTarget(target, defaultRoot, info.IsDir(), conflict); err != nil {
			return err
		}
	}
	targetRoot := state.target(rootKey, func() string {
		return resolveRemoteCopyTarget(target, defaultRoot, info.IsDir(), conflict)
	})
	if targetRoot == "" {
		report(0, 0)
		return nil
	}

	plan, err := buildRemoteCopyPlan(ctx, source, sourceRoot, targetRoot)
	if err != nil {
		return fmt.Errorf("读取源目录: %w", err)
	}
	var done int64
	for _, item := range plan.items {
		if !item.dir && state.done(item.source) {
			done += item.size
		}
	}
	report(done, plan.total)

	for _, item := range plan.items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if item.dir {
			if err := target.MkdirAll(item.target); err != nil {
				return fmt.Errorf("创建目标目录 %s: %w", item.target, err)
			}
			if item.mode.Perm() != 0 {
				_ = target.Chmod(item.target, item.mode.Perm())
			}
			continue
		}
		if state.done(item.source) {
			continue
		}

		key := "remote:" + item.source
		if _, cached := state.targets[key]; !cached {
			if err := validateRemoteCopyTarget(target, item.target, false, conflict); err != nil {
				return err
			}
		}
		targetPath := state.target(key, func() string {
			return resolveRemoteCopyTarget(target, item.target, false, conflict)
		})
		if targetPath == "" {
			state.complete(item.source)
			done += item.size
			report(done, plan.total)
			continue
		}

		base := done
		if err := copyRemoteFileResume(ctx, source, target, item.source, targetPath, func(n, _ int64) {
			report(base+n, plan.total)
		}); err != nil {
			return fmt.Errorf("复制 %s: %w", path.Base(item.source), err)
		}
		state.complete(item.source)
		done += item.size
		report(done, plan.total)
	}
	return nil
}

func resolveRemoteCopyTarget(target *sftp.Client, desired string, sourceDir bool, conflict transferConflict) string {
	if !remotePathExists(target, desired) {
		return desired
	}
	switch conflict {
	case transferConflictSkip:
		if sourceDir {
			if info, err := target.Stat(desired); err == nil && info.IsDir() {
				return desired
			}
		}
		return ""
	case transferConflictRename:
		return uniqueRemotePath(target, desired)
	default:
		return desired
	}
}

func validateRemoteCopyTarget(target *sftp.Client, desired string, sourceDir bool, conflict transferConflict) error {
	if conflict != transferConflictOverwrite {
		return nil
	}
	info, err := target.Stat(desired)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() != sourceDir {
		if sourceDir {
			return fmt.Errorf("目标位置已是文件: %s", desired)
		}
		return fmt.Errorf("目标位置已是目录: %s", desired)
	}
	return nil
}

func copyRemoteFileResume(ctx context.Context, source, target *sftp.Client, sourcePath, targetPath string, report func(int64, int64)) error {
	src, err := source.Open(sourcePath)
	if err != nil {
		return err
	}
	defer src.Close()

	info, err := src.Stat()
	if err != nil {
		return err
	}
	size := info.Size()

	tmp := targetPath + ".xu-part"
	var offset int64
	partExists := false
	if part, err := target.Stat(tmp); err == nil {
		partExists = true
		if part.Size() > size {
			_ = target.Remove(tmp)
			partExists = false
		} else {
			offset = part.Size()
		}
	}
	if offset > 0 {
		if _, err := src.Seek(offset, io.SeekStart); err != nil {
			return err
		}
	}

	if offset < size || !partExists {
		flags := os.O_WRONLY | os.O_CREATE
		if !partExists {
			flags |= os.O_TRUNC
		}
		dst, err := target.OpenFile(tmp, flags)
		if err != nil {
			return err
		}
		if offset > 0 {
			if _, err := dst.Seek(offset, io.SeekStart); err != nil {
				dst.Close()
				return err
			}
		}
		report(offset, size)
		if err := copyWithProgressFrom(ctx, dst, src, size, offset, report); err != nil {
			dst.Close()
			return err
		}
		if err := dst.Close(); err != nil {
			return err
		}
	} else {
		report(size, size)
	}

	if st, err := target.Stat(tmp); err != nil {
		return err
	} else if st.Size() != size {
		return fmt.Errorf("复制不完整: %d/%d 字节", st.Size(), size)
	}

	if err := target.PosixRename(tmp, targetPath); err != nil {
		_ = target.Remove(targetPath)
		if err2 := target.Rename(tmp, targetPath); err2 != nil {
			return err2
		}
	}
	if info.Mode().Perm() != 0 {
		_ = target.Chmod(targetPath, info.Mode().Perm())
	}
	return nil
}
