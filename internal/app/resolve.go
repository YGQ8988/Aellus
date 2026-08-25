package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// resolveDir 校验并解析目录（支持多级子目录）。
// name 为相对 saveDir 的路径，各层用 "/" 分隔，例如 "设备名/子目录/深层"。
// 空串对应 file-drops 根目录（未分子目录，即「未命名设备」）。
func (a *App) resolveDir(name string) (string, error) {
	// 空串对应 file-drops 根目录（未分子目录）。
	if name == "" {
		return a.getSaveDir(), nil
	}
	// 统一分隔符，逐层校验，杜绝路径穿越。
	clean := strings.ReplaceAll(name, "\\", "/")
	clean = strings.Trim(clean, "/")
	segs := strings.Split(clean, "/")
	for _, s := range segs {
		if !isValidName(s) {
			return "", errors.New("非法目录名")
		}
	}
	full := filepath.Join(a.getSaveDir(), filepath.Join(segs...))
	if !isInside(a.getSaveDir(), full) {
		return "", errors.New("路径穿越")
	}
	// symlink 增强：解析真实路径后再次确认仍在 saveDir 内，杜绝授权目录内的软链逃逸。
	if !realInside(a.getSaveDir(), full) {
		return "", errors.New("路径穿越")
	}
	info, err := os.Stat(full)
	if err != nil || !info.IsDir() {
		return "", errors.New("目录不存在")
	}
	return full, nil
}

// resolveFile 校验并解析单个文件，返回其绝对路径（要求文件确实存在）。
func (a *App) resolveFile(dirAbs, name string) (string, error) {
	if !isValidName(name) {
		return "", errors.New("非法文件名")
	}
	full := filepath.Join(dirAbs, name)
	if !isInside(dirAbs, full) {
		return "", errors.New("路径穿越")
	}
	// symlink 增强：解析真实路径后再次确认，防止软链文件指向目录外。
	if !realInside(dirAbs, full) {
		return "", errors.New("路径穿越")
	}
	if _, err := os.Stat(full); err != nil {
		return "", errors.New("文件不存在")
	}
	return full, nil
}
