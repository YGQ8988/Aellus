package app

// === 数据结构 ===
// Go 的 html/template 只能访问【大写字母开头】的字段，所以这里全部大写开头。
// JSON 的 key 由 `json:"..."` tag 决定（前端 JS 里写死的字段名）。

// —— 上传返回 ——
type UploadFileResp struct {
	Name  string `json:"name"`  // 保存到磁盘的文件名
	Size  int64  `json:"size"`  // 字节数
	Mtime int64  `json:"mtime"` // 修改时间（Unix 时间戳，与 /api/files 的 FileInfo 一致，前端格式化）
}
type UploadResp struct {
	OK      bool             `json:"ok"`
	Files   []UploadFileResp `json:"files"`
	Dir     string           `json:"dir"`     // 设备目录名
	Message string           `json:"message"` // 失败时的原因（含出错文件名），便于排查
}

// —— 目录列表返回 ——
type DirInfo struct {
	Name      string `json:"name"`      // 设备目录名
	Count     int    `json:"count"`     // 目录下（仅一层）非隐藏文件数量
	Size      int64  `json:"size"`      // 目录下非隐藏文件总大小（字节）
	Mtime     int64  `json:"mtime"`     // 目录下最新修改时间（Unix 时间戳）
	Deletable bool   `json:"deletable"` // 当前请求是否可删除该目录（服务端按 IP/UA 归属判定）
}
type DirsResp struct {
	Dirs []DirInfo `json:"dirs"`
}

// —— 文件列表返回 ——
type FileInfo struct {
	Name      string `json:"name"`      // 文件名 / 文件夹名
	Size      int64  `json:"size"`      // 字节数（文件夹为递归总大小）
	Mtime     int64  `json:"mtime"`     // 修改时间（Unix 时间戳，int64，前端再格式化成可读时间）
	IsDir     bool   `json:"isDir"`     // true 表示该项是一个子目录（可继续进入）
	Count     int    `json:"count"`     // 文件夹内文件总数（仅文件夹有意义，文件为 0）
	Deletable bool   `json:"deletable"` // 当前请求是否可删除该项（服务端按 IP/UA 归属判定）
}
type FilesResp struct {
	Dir   string     `json:"dir"`
	Files []FileInfo `json:"files"`
	Error string     `json:"error,omitempty"` // 出错时填充，前端 browse.js 会读这个字段
}

// —— 批量下载请求体 ——
type BatchReq struct {
	Dir   string   `json:"dir"`   // 设备目录名
	Files []string `json:"files"` // 要打包的文件名；为空表示打包该目录全部非隐藏文件
}
