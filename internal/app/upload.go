package app

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
)

type uploadItem struct {
	local  string
	remote string
	size   int64
	dir    bool
}

type uploadPlan struct {
	items []uploadItem
	total int64
}

func (a *App) uploadFolderViaDialog(b *fileBrowser) {
	if b == nil || b.client == nil {
		a.fileErr = "需要 SSH 连接才能上传"
		return
	}

	dirs, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:      a.win,
		Title:       "选择要上传的文件夹",
		ButtonLabel: "上传",
		Directory:   true,
	})
	if err != nil || len(dirs) == 0 {
		return
	}
	a.uploadPaths(b, dirs)
}

func (a *App) uploadDropped(b *fileBrowser, paths []string) {
	a.uploadPaths(b, paths)
}

func (a *App) uploadPaths(b *fileBrowser, paths []string) {
	if b == nil {
		a.fileErr = "文件面板已关闭，请重试"
		return
	}
	a.uploadPathsTo(b, paths, b.remoteDir)
}

func (a *App) uploadPathsTo(b *fileBrowser, paths []string, remoteDir string) {
	if b == nil || b.client == nil {
		a.fileErr = "需要 SSH 连接才能上传"
		return
	}
	paths = compactUploadPaths(paths)
	if len(paths) == 0 {
		return
	}

	remoteDir = path.Clean(remoteDir)
	if remoteDir == "." || remoteDir == "" || !strings.HasPrefix(remoteDir, "/") {
		a.fileErr = "远端目录无效"
		return
	}

	label := "上传 " + filepath.Base(paths[0])
	if len(paths) > 1 {
		label = fmt.Sprintf("上传 %d 个项目", len(paths))
	}

	a.fileErr = ""
	a.enqueueTransferJob(transferUpload, label, remoteDir, b.termID, transferPayload{
		Conflict: int(a.transferConflictPolicy()),
		Upload: &transferUploadPayload{
			Paths:     paths,
			RemoteDir: remoteDir,
		},
	})
}

func compactUploadPaths(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		p = filepath.Clean(p)
		key := p
		if os.PathSeparator == '\\' {
			key = strings.ToLower(key)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}

// buildUploadPlan walks every selected local file or folder before starting,
// so the transfer can report one total and folders keep their original tree.
func buildUploadPlan(paths []string, remoteDir string) (uploadPlan, error) {
	var plan uploadPlan
	remoteDir = path.Clean(remoteDir)
	if remoteDir == "." || remoteDir == "" || !strings.HasPrefix(remoteDir, "/") {
		return plan, fmt.Errorf("远端目录无效")
	}

	names := make(map[string]bool, len(paths))
	for _, local := range compactUploadPaths(paths) {
		if filepath.Dir(local) == local {
			return uploadPlan{}, fmt.Errorf("不能上传磁盘根目录: %s", local)
		}
		base := filepath.Base(local)
		if !validName(base) {
			return uploadPlan{}, fmt.Errorf("无法上传 %s：名称为空或包含无效字符", local)
		}
		if names[base] {
			return uploadPlan{}, fmt.Errorf("所选项目包含同名文件或文件夹: %s", base)
		}
		names[base] = true

		info, err := os.Lstat(local)
		if err != nil {
			return uploadPlan{}, fmt.Errorf("读取 %s: %w", local, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return uploadPlan{}, fmt.Errorf("暂不跟随符号链接: %s", local)
		}

		if !info.IsDir() {
			if !info.Mode().IsRegular() {
				return uploadPlan{}, fmt.Errorf("不是普通文件: %s", local)
			}
			plan.items = append(plan.items, uploadItem{
				local:  local,
				remote: path.Join(remoteDir, base),
				size:   info.Size(),
			})
			plan.total += info.Size()
			continue
		}

		remoteRoot := path.Join(remoteDir, base)
		err = filepath.WalkDir(local, func(p string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nil
			}

			rel, err := filepath.Rel(local, p)
			if err != nil {
				return err
			}
			remote := remoteRoot
			if rel != "." {
				remote = path.Join(remoteRoot, filepath.ToSlash(rel))
			}
			if d.IsDir() {
				plan.items = append(plan.items, uploadItem{remote: remote, dir: true})
				return nil
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			plan.items = append(plan.items, uploadItem{
				local:  p,
				remote: remote,
				size:   info.Size(),
			})
			plan.total += info.Size()
			return nil
		})
		if err != nil {
			return uploadPlan{}, fmt.Errorf("读取 %s: %w", local, err)
		}
	}
	if len(plan.items) == 0 {
		return plan, fmt.Errorf("没有可上传的文件")
	}
	return plan, nil
}
