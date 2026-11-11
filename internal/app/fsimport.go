package app

import (
	"bytes"
	"crypto/des"
	"crypto/md5"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"

	"xu/internal/store"
	"xu/internal/vault"
)

// FinalShell exports one connection per JSON file when "导出" is used on a
// connection. The fields below are the ones we import; the rest are ignored.
type finalShellConn struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	UserName string `json:"user_name"`
	Password string `json:"password"`
	AuthType int    `json:"authentication_type"`
	ConnType int    `json:"conection_type"` // FinalShell's spelling
}

type finalShellSummary struct {
	Files      int
	Imported   int
	Skipped    int
	Failed     int
	NoPassword int
}

func (s finalShellSummary) notice() string {
	parts := []string{}
	if s.Imported > 0 {
		parts = append(parts, fmt.Sprintf("已导入 %d 个连接", s.Imported))
	} else {
		parts = append(parts, "没有可导入的连接")
	}
	if s.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("跳过 %d 个重复/不支持项", s.Skipped))
	}
	if s.Failed > 0 {
		parts = append(parts, fmt.Sprintf("失败 %d 个", s.Failed))
	}
	if s.NoPassword > 0 {
		parts = append(parts, fmt.Sprintf("%d 个密码未导入", s.NoPassword))
	}
	return strings.Join(parts, " · ")
}

// The file picker and the keychain are variables so tests can stand in for
// them without opening a native dialog or touching the real credentials.
var openFinalShellImport = func(opts mygo.OpenDialogOptions) ([]string, error) {
	return mygo.Dialog.Open(opts)
}

var setImportedPassword = vault.Set

// importFinalShell asks for one or more FinalShell exports and saves the
// connections they contain.
func (a *App) importFinalShell() {
	opts := mygo.OpenDialogOptions{
		Title:    "导入 FinalShell 连接（可多选）",
		Filters:  []mygo.FileFilter{{Name: "FinalShell 导出的 JSON", Extensions: []string{"json"}}},
		Multiple: true,
	}
	if home, err := os.UserHomeDir(); err == nil {
		dir := filepath.Join(home, "Downloads")
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			opts.DefaultPath = dir
		}
	}

	paths, err := openFinalShellImport(opts)
	if err != nil {
		a.managerErr = "打开文件选择器失败: " + err.Error()
		return
	}
	if len(paths) == 0 {
		return
	}
	if a.store == nil {
		a.managerErr = "会话库未就绪，暂时不能导入连接"
		return
	}

	a.managerErr = ""
	a.managerNotice = ""
	sum := a.importFinalShellPaths(paths)
	a.refreshProfiles()
	a.refreshGroups()
	a.managerNotice = sum.notice()
}

// importFinalShellPaths parses paths and saves their SSH connections. It
// never lets one bad file stop the rest of the import.
func (a *App) importFinalShellPaths(paths []string) finalShellSummary {
	sum := finalShellSummary{}
	existing := make(map[string]bool, len(a.profiles)+len(paths))
	for _, p := range a.profiles {
		existing[fsProfileKey(p.Host, p.Port, p.Username)] = true
	}

	for _, path := range paths {
		sum.Files++
		data, err := os.ReadFile(path)
		if err != nil {
			sum.Failed++
			continue
		}
		conns, err := parseFinalShellExport(data)
		if err != nil {
			sum.Failed++
			continue
		}
		for _, c := range conns {
			host := strings.TrimSpace(c.Host)
			user := strings.TrimSpace(c.UserName)
			// FinalShell's conection_type 100 is SSH; anything else is a
			// protocol this app cannot open.
			if host == "" || user == "" || (c.ConnType != 0 && c.ConnType != 100) {
				sum.Skipped++
				continue
			}
			port := c.Port
			if port <= 0 || port > 65535 {
				port = 22
			}
			key := fsProfileKey(host, port, user)
			if existing[key] {
				sum.Skipped++
				continue
			}

			password := ""
			if c.Password != "" {
				if pw, err := decodeFinalShellPassword(c.Password); err == nil {
					password = pw
				} else {
					sum.NoPassword++
				}
			}
			name := strings.TrimSpace(c.Name)
			if name == "" {
				name = user + "@" + host
			}
			authType := "key"
			if password != "" {
				authType = "password"
			}

			prof := store.Profile{
				Name:     name,
				Host:     host,
				Port:     port,
				Username: user,
				AuthType: authType,
			}
			if err := a.store.SaveProfile(a.ctx, &prof); err != nil {
				sum.Failed++
				continue
			}
			if password != "" {
				if err := setImportedPassword(prof.ID, "password", password); err != nil {
					sum.NoPassword++
				}
			}
			existing[key] = true
			sum.Imported++
		}
	}
	return sum
}

func fsProfileKey(host string, port int, user string) string {
	return strings.ToLower(strings.TrimSpace(host)) + ":" +
		fmt.Sprintf("%d", port) + ":" + strings.ToLower(strings.TrimSpace(user))
}

// parseFinalShellExport accepts the single-connection object FinalShell
// writes and, defensively, an array of those objects.
func parseFinalShellExport(data []byte) ([]finalShellConn, error) {
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}))
	if len(data) == 0 {
		return nil, errors.New("JSON 文件为空")
	}
	switch data[0] {
	case '[':
		var list []finalShellConn
		if err := json.Unmarshal(data, &list); err != nil {
			return nil, fmt.Errorf("读取 FinalShell 导出失败: %w", err)
		}
		return list, nil
	case '{':
		var conn finalShellConn
		if err := json.Unmarshal(data, &conn); err != nil {
			return nil, fmt.Errorf("读取 FinalShell 导出失败: %w", err)
		}
		return []finalShellConn{conn}, nil
	}
	return nil, errors.New("不是 FinalShell 导出的连接 JSON")
}

// decodeFinalShellPassword decrypts FinalShell's exported password. The
// base64 blob holds an arbitrary 8-byte "head" followed by DES/ECB data; the
// DES key is a deterministic MD5 of values derived from the head with Java's
// java.util.Random, so the random sequence is reproduced here exactly.
//
// The scheme is the one used by the community's FinalShellDecodePass tool.
func decodeFinalShellPassword(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", fmt.Errorf("密码不是有效的 Base64: %w", err)
	}
	if len(raw) < 16 || (len(raw)-8)%8 != 0 {
		return "", errors.New("密码数据长度不正确")
	}
	head, ct := raw[:8], raw[8:]

	div := newJavaRandom(int64(int8(head[5]))).nextInt(127)
	if div == 0 {
		return "", errors.New("密码密钥无效")
	}
	r1 := newJavaRandom(int64(3680984568597093857) / int64(div))
	for i := 0; i < int(int8(head[0])); i++ {
		r1.nextLong()
	}
	r2 := newJavaRandom(r1.nextLong())
	longs := []int64{
		int64(int8(head[4])), r2.nextLong(), int64(int8(head[7])), int64(int8(head[3])),
		r2.nextLong(), int64(int8(head[1])), r1.nextLong(), int64(int8(head[2])),
	}
	keyData := make([]byte, 64)
	for i, v := range longs {
		binary.BigEndian.PutUint64(keyData[i*8:], uint64(v))
	}
	sum := md5.Sum(keyData)
	block, err := des.NewCipher(sum[:8])
	if err != nil {
		return "", err
	}

	out := make([]byte, len(ct))
	for i := 0; i < len(ct); i += 8 {
		block.Decrypt(out[i:i+8], ct[i:i+8])
	}
	// DES/ECB/PKCS5Padding, as Java's Cipher.getInstance("DES").
	n := int(out[len(out)-1])
	if n < 1 || n > 8 || n > len(out) {
		return "", errors.New("密码填充无效")
	}
	for _, b := range out[len(out)-n:] {
		if int(b) != n {
			return "", errors.New("密码填充无效")
		}
	}
	return string(out[:len(out)-n]), nil
}

// javaRandom reproduces java.util.Random's 48-bit linear congruential
// generator, which FinalShell's key derivation depends on bit for bit.
type javaRandom struct{ seed int64 }

func newJavaRandom(seed int64) *javaRandom {
	return &javaRandom{seed: (seed ^ 0x5DEECE66D) & ((1 << 48) - 1)}
}

func (r *javaRandom) next(bits uint) int32 {
	r.seed = (r.seed*0x5DEECE66D + 0xB) & ((1 << 48) - 1)
	return int32(uint64(r.seed) >> (48 - bits))
}

func (r *javaRandom) nextInt(bound int32) int32 {
	if bound <= 0 {
		return 0
	}
	if bound&(-bound) == bound {
		return int32((int64(bound) * int64(r.next(31))) >> 31)
	}
	for {
		bits := r.next(31)
		val := bits % bound
		if bits-val+(bound-1) >= 0 {
			return val
		}
	}
}

func (r *javaRandom) nextLong() int64 {
	return (int64(r.next(32)) << 32) + int64(r.next(32))
}
