package app

import (
	"net/http"
	"strconv"
)

// pageScope 解析分页参数（?page=1&size=30）并返回本页切片与分页信息。
//
//   - size 未传 / 非法时返回全量（size=0），保持接口向后兼容（不带参数 = 旧行为）；
//   - page 越界时返回空切片（total 仍为真实总数，前端据此回退到最后一页）；
//   - size 上限 1000，防单次拉取过大。
//
// 切片在调用方排序之后进行（顺序稳定，跨页不重不漏）。
func pageScope[T any](r *http.Request, all []T) (part []T, total, page, size int) {
	total = len(all)
	page = 1
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := r.URL.Query().Get("size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			size = n
		}
	}
	if size > 1000 {
		size = 1000
	}
	if size == 0 {
		return all, total, 1, 0
	}
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	part = all[start:end]
	if part == nil {
		part = []T{} // 空页输出 [] 而非 null（前端判 .length）
	}
	return part, total, page, size
}
