// Package install registers cronctl into the user PATH (~/.local/bin).
// Install is intentionally cwd-first so re-installing from another checkout
// overwrites a previous registration (env + wrappers), instead of sticking to
// a stale SPROUTAI_CRON_ROOT.
package install

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const tasksDirName = "cron-tasks"

// Result is a human-readable summary of what install did.
type Result struct {
	Root       string   `json:"root"`
	RootHow    string   `json:"root_how"`
	Binary     string   `json:"binary"`
	BinDir     string   `json:"bin_dir"`
	Wrote      []string `json:"wrote"`
	Notes      []string `json:"notes"`
	PathOK     bool     `json:"path_ok"`
	EnvRoot    string   `json:"env_root,omitempty"`
	PrevRoot   string   `json:"prev_root,omitempty"`
	Overwrote  bool     `json:"overwrote"`
}

// Options configures install.
type Options struct {
	// Root is an explicit project root (--root). Empty → auto-detect.
	Root string
	// PreferSelf forces using the currently running executable when possible.
	PreferSelf bool
}

// Run registers the cronctl binary for the current user (always overwrites wrappers).
func Run(opts Options) (*Result, error) {
	cronRoot, how, err := ResolveRoot(opts.Root)
	if err != nil {
		return nil, err
	}
	cronRoot, err = filepath.Abs(cronRoot)
	if err != nil {
		return nil, err
	}

	prevRoot := strings.TrimSpace(os.Getenv("SPROUTAI_CRON_ROOT"))
	if prevRoot != "" {
		if abs, err := filepath.Abs(prevRoot); err == nil {
			prevRoot = abs
		}
	}

	bin, err := resolveBinary(cronRoot, opts.PreferSelf)
	if err != nil {
		return nil, err
	}
	bin, err = filepath.Abs(bin)
	if err != nil {
		return nil, err
	}

	binDir, err := userBinDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建 bin 目录失败: %w", err)
	}

	res := &Result{
		Root:     cronRoot,
		RootHow:  how,
		Binary:   bin,
		BinDir:   binDir,
		PrevRoot: prevRoot,
	}
	if prevRoot != "" && !samePath(prevRoot, cronRoot) {
		res.Overwrote = true
		res.Notes = append(res.Notes,
			fmt.Sprintf("覆盖安装: SPROUTAI_CRON_ROOT %s → %s", prevRoot, cronRoot),
		)
	} else if prevRoot != "" {
		res.Overwrote = true
		res.Notes = append(res.Notes, "覆盖安装: 更新已有注册（包装器 / 二进制副本 / 环境变量）")
	}

	if runtime.GOOS == "windows" {
		if err := installWindows(res, cronRoot, bin, binDir); err != nil {
			return res, err
		}
	} else {
		if err := installUnix(res, cronRoot, bin, binDir); err != nil {
			return res, err
		}
	}

	pathAdded, err := ensureUserPath(binDir)
	if err != nil {
		res.Notes = append(res.Notes, fmt.Sprintf("PATH 更新失败: %v（请手动加入 %s）", err, binDir))
	} else {
		res.PathOK = true
		if pathAdded {
			res.Notes = append(res.Notes, "已将 bin 目录加入用户 PATH（请新开终端）")
		} else {
			res.Notes = append(res.Notes, "bin 目录已在用户 PATH 中")
		}
	}

	if err := setUserEnv("SPROUTAI_CRON_ROOT", cronRoot); err != nil {
		res.Notes = append(res.Notes, fmt.Sprintf("设置 SPROUTAI_CRON_ROOT 失败: %v", err))
	} else {
		res.EnvRoot = cronRoot
		res.Notes = append(res.Notes, "已设置用户环境变量 SPROUTAI_CRON_ROOT（请新开终端）")
	}

	return res, nil
}

// ResolveRoot finds the project root for install.
// Order: explicit --root → cwd walk-up → executable walk-up (skip ~/.local/bin) → env (last resort).
// Intentionally does NOT prefer env first, so reinstall from another checkout works.
func ResolveRoot(explicit string) (root, how string, err error) {
	if strings.TrimSpace(explicit) != "" {
		abs, err := filepath.Abs(explicit)
		if err != nil {
			return "", "", err
		}
		if !isProjectRoot(abs) {
			return "", "", fmt.Errorf("--root 不是有效项目根（缺少 %s/）: %s", tasksDirName, abs)
		}
		return abs, "flag --root", nil
	}

	if cwd, err := os.Getwd(); err == nil {
		if r, ok := walkUpRoot(cwd); ok {
			return r, "cwd", nil
		}
	}

	if exe, err := stableExecutable(); err == nil {
		dir := filepath.Dir(exe)
		if !isUserLocalBin(dir) {
			if r, ok := walkUpRoot(dir); ok {
				return r, "executable", nil
			}
			// dist/windows-amd64/cronctl.exe → repo root is ../..
			if r, ok := walkUpRoot(filepath.Dir(dir)); ok {
				return r, "executable (dist parent)", nil
			}
		}
	}

	// Last resort only: previous install env (warn via how string)
	for _, key := range []string{"SPROUTAI_CRON_ROOT", "CRON_ROOT"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			abs, err := filepath.Abs(v)
			if err != nil {
				continue
			}
			if isProjectRoot(abs) {
				return abs, "env " + key + "（当前目录不是项目根，回退到环境变量）", nil
			}
		}
	}

	return "", "", fmt.Errorf(
		"无法定位 sproutai-cron 项目根（需含 %s/）。请在仓库目录内执行 cronctl install，或指定 --root <路径>",
		tasksDirName,
	)
}

func walkUpRoot(start string) (string, bool) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		if isProjectRoot(dir) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func isProjectRoot(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, tasksDirName))
	return err == nil && st.IsDir()
}

func isUserLocalBin(dir string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	localBin := filepath.Join(home, ".local", "bin")
	return samePath(dir, localBin)
}

func samePath(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(aa, bb)
	}
	return aa == bb
}

func isUnder(root, path string) bool {
	rootAbs, err1 := filepath.Abs(root)
	pathAbs, err2 := filepath.Abs(path)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

func userBinDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin"), nil
}

func resolveBinary(cronRoot string, preferSelf bool) (string, error) {
	self, selfErr := stableExecutable()
	selfUnderRoot := selfErr == nil && isUnder(cronRoot, self) && !isUserLocalBin(filepath.Dir(self))

	// If user ran ./cronctl.exe install inside the project, install THAT binary.
	if selfUnderRoot {
		return self, nil
	}
	if preferSelf && selfErr == nil && !isUserLocalBin(filepath.Dir(self)) {
		return self, nil
	}

	var candidates []string
	switch runtime.GOOS {
	case "windows":
		candidates = []string{
			filepath.Join(cronRoot, "dist", "windows-amd64", "cronctl.exe"),
			filepath.Join(cronRoot, "cronctl.exe"),
			filepath.Join(cronRoot, "bin", "cronctl.exe"),
		}
	default:
		candidates = []string{
			filepath.Join(cronRoot, "dist", "linux-amd64", "cronctl"),
			filepath.Join(cronRoot, "bin", "cronctl"),
			filepath.Join(cronRoot, "cronctl"),
		}
		if runtime.GOOS == "darwin" {
			candidates = append([]string{
				filepath.Join(cronRoot, "bin", "cronctl"),
			}, candidates...)
		}
	}

	for _, c := range candidates {
		if fileExists(c) {
			return c, nil
		}
	}

	if selfErr == nil && fileExists(self) && !isUserLocalBin(filepath.Dir(self)) {
		return self, nil
	}

	if runtime.GOOS == "windows" {
		return "", fmt.Errorf("未找到 cronctl 二进制。请先在项目内: build.cmd  或  go build -o cronctl.exe ./cmd/cronctl")
	}
	return "", fmt.Errorf("未找到 cronctl 二进制。请先在项目内: ./build.sh  或  go build -o cronctl ./cmd/cronctl")
}

// stableExecutable returns os.Executable after symlink eval; skips go-run temp when possible.
func stableExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	low := strings.ToLower(exe)
	sep := string(filepath.Separator)
	if strings.Contains(low, sep+"go-build") {
		return "", fmt.Errorf("temporary go run binary")
	}
	return exe, nil
}

func installWindows(res *Result, cronRoot, bin, binDir string) error {
	// Wrappers always overwrite existing files (覆盖安装).

	// 1) cronctl.cmd for CMD / PowerShell
	cmdPath := filepath.Join(binDir, "cronctl.cmd")
	cmdBody := fmt.Sprintf(
		"@echo off\r\nsetlocal EnableExtensions\r\nset \"SPROUTAI_CRON_ROOT=%s\"\r\nchcp 65001 >nul 2>&1\r\n\"%s\" %%*\r\n",
		cronRoot, bin,
	)
	if err := os.WriteFile(cmdPath, []byte(cmdBody), 0o644); err != nil {
		return err
	}
	res.Wrote = append(res.Wrote, cmdPath)

	// 2) copy as cronctl.exe so Git Bash resolves `cronctl` / `cronctl.exe`
	dstExe := filepath.Join(binDir, "cronctl.exe")
	if err := copyFile(bin, dstExe); err != nil {
		res.Notes = append(res.Notes, fmt.Sprintf("复制 cronctl.exe 失败: %v", err))
	} else {
		res.Wrote = append(res.Wrote, dstExe)
	}

	// 3) bash wrapper (no extension) for Git Bash / MSYS
	shPath := filepath.Join(binDir, "cronctl")
	rootUnix := filepath.ToSlash(cronRoot)
	binUnix := filepath.ToSlash(bin)
	shBody := fmt.Sprintf("#!/usr/bin/env bash\nexport SPROUTAI_CRON_ROOT=\"%s\"\nexec \"%s\" \"$@\"\n", rootUnix, binUnix)
	if err := os.WriteFile(shPath, []byte(shBody), 0o755); err != nil {
		return err
	}
	res.Wrote = append(res.Wrote, shPath)

	res.Notes = append(res.Notes,
		"CMD/PowerShell: cronctl",
		"Git Bash: cronctl 或 cronctl.exe",
	)
	return nil
}

func installUnix(res *Result, cronRoot, bin, binDir string) error {
	dst := filepath.Join(binDir, "cronctl")
	body := fmt.Sprintf("#!/usr/bin/env bash\nexport SPROUTAI_CRON_ROOT=%q\nexec %q \"$@\"\n", cronRoot, bin)
	if err := os.WriteFile(dst, []byte(body), 0o755); err != nil {
		return err
	}
	_ = os.Chmod(bin, 0o755)
	res.Wrote = append(res.Wrote, dst)
	res.Notes = append(res.Notes, "Unix: cronctl")
	return nil
}

func copyFile(src, dst string) error {
	// If src and dst are the same file, nothing to do.
	if samePath(src, dst) {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	_ = os.Remove(dst)
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// Print writes a human summary to w.
func Print(w io.Writer, res *Result) {
	if res == nil {
		return
	}
	fmt.Fprintf(w, "[install] project root: %s  (%s)\n", res.Root, res.RootHow)
	fmt.Fprintf(w, "[install] binary: %s\n", res.Binary)
	fmt.Fprintf(w, "[install] bin dir: %s\n", res.BinDir)
	for _, p := range res.Wrote {
		fmt.Fprintf(w, "[install] wrote %s\n", p)
	}
	for _, n := range res.Notes {
		fmt.Fprintf(w, "[install] %s\n", n)
	}
	fmt.Fprintln(w, "[install] done. open a NEW terminal, then: cronctl status")
}
