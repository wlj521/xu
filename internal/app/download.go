package app

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/pkg/sftp"
)

type downloadItem struct {
	remote string
	local  string
	size   int64
	dir    bool
}

type downloadPlan struct {
	items []downloadItem
	total int64
}

var openDownloadFolderDialog = func(opts mygo.OpenDialogOptions) ([]string, error) {
	return mygo.Dialog.Open(opts)
}

func (a *App) chooseDownloadFolder(b *fileBrowser, f os.FileInfo) (string, string, error) {
	dirs, err := openDownloadFolderDialog(mygo.OpenDialogOptions{
		Parent:      a.win,
		Title:       "选择文件夹下载位置",
		ButtonLabel: "保存到这里",
		DefaultPath: b.localDir,
		Directory:   true,
	})
	if err != nil || len(dirs) == 0 {
		return "", "", err
	}
	parent := filepath.Clean(dirs[0])
	return parent, filepath.Join(parent, safeLocalName(f.Name())), nil
}

func buildDownloadPlan(client *sftp.Client, remoteRoot, localRoot string) (downloadPlan, error) {
	plan := downloadPlan{
		items: []downloadItem{{remote: remoteRoot, local: localRoot, dir: true}},
	}
	if err := appendRemoteTree(client, remoteRoot, localRoot, &plan); err != nil {
		return downloadPlan{}, err
	}
	return plan, nil
}

func appendRemoteTree(client *sftp.Client, remoteDir, localDir string, plan *downloadPlan) error {
	entries, err := client.ReadDir(remoteDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Mode()&os.ModeSymlink != 0 {
			continue
		}
		remote := path.Join(remoteDir, entry.Name())
		local := filepath.Join(localDir, entry.Name())
		switch {
		case entry.IsDir():
			plan.items = append(plan.items, downloadItem{remote: remote, local: local, dir: true})
			if err := appendRemoteTree(client, remote, local, plan); err != nil {
				return err
			}
		case entry.Mode().IsRegular():
			plan.items = append(plan.items, downloadItem{
				remote: remote,
				local:  local,
				size:   entry.Size(),
			})
			plan.total += entry.Size()
		}
	}
	return nil
}

func downloadDirectory(ctx context.Context, client *sftp.Client, remoteRoot, localRoot string, conflict transferConflict, report func(int64, int64)) error {
	return downloadDirectoryWithState(ctx, client, remoteRoot, localRoot, conflict, report, newTransferResumeState())
}

func downloadDirectoryWithState(ctx context.Context, client *sftp.Client, remoteRoot, localRoot string, conflict transferConflict, report func(int64, int64), state *transferResumeState) error {
	if report == nil {
		report = func(int64, int64) {}
	}
	if state == nil {
		state = newTransferResumeState()
	}
	localRoot = state.target("download-root:"+remoteRoot, func() string {
		if conflict == transferConflictRename {
			return uniqueLocalPath(localRoot)
		}
		return localRoot
	})
	plan, err := buildDownloadPlan(client, remoteRoot, localRoot)
	if err != nil {
		return fmt.Errorf("读取远端目录: %w", err)
	}

	var done int64
	for _, item := range plan.items {
		if !item.dir && state.done(item.remote) {
			done += item.size
		}
	}
	report(done, plan.total)

	for _, item := range plan.items {
		if err := ctx.Err(); err != nil {
			return err
		}
		if item.dir {
			if err := os.MkdirAll(item.local, 0o755); err != nil {
				return fmt.Errorf("创建本地目录 %s: %w", item.local, err)
			}
			continue
		}
		if state.done(item.remote) {
			continue
		}

		target := state.target("download:"+item.remote, func() string {
			if !localPathExists(item.local) {
				return item.local
			}
			switch conflict {
			case transferConflictSkip:
				return ""
			case transferConflictRename:
				return uniqueLocalPath(item.local)
			default:
				return item.local
			}
		})
		if target == "" {
			state.complete(item.remote)
			done += item.size
			report(done, plan.total)
			continue
		}

		base := done
		if err := downloadFileResume(ctx, client, item.remote, target, func(n, _ int64) {
			report(base+n, plan.total)
		}); err != nil {
			return fmt.Errorf("下载 %s: %w", path.Base(item.remote), err)
		}
		state.complete(item.remote)
		done += item.size
		report(done, plan.total)
	}
	return nil
}
