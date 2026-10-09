// 上传页逻辑（$ / formatSize / formatTime 等通用工具来自 ui.js 共享层）

// 记住设备名称：上传时存 localStorage，下次打开上传页自动填充，避免每次重新输入
const DEVICE_NAME_KEY = 'aellus_device_name';
function restoreDeviceName() {
  try {
    var saved = localStorage.getItem(DEVICE_NAME_KEY);
    if (saved) { $('device').value = saved; }
  } catch (e) {}
  // 不再从服务端 /api/settings 取回设备名：服务端按设备 ID 记录的名字无法在前端清除
  //（清掉 localStorage 后刷新仍会被服务端回填），表现为“填了清不掉”。改为只依赖本机
  // localStorage 记忆，清空输入框即删除记忆，下次打开不再自动恢复。
}
function rememberDeviceName(name) {
  try { localStorage.setItem(DEVICE_NAME_KEY, name); } catch (e) {}
}

// 上传速度格式化：始终以 MB/s 显示（不上 GB/s，避免单位跳变且局域网内极少突破 1GB/s）
function formatSpeed(bps) {
  const mbps = bps / 1048576;
  if (mbps < 0.1) return (mbps * 1000).toFixed(0) + ' KB/s';
  return mbps.toFixed(1) + ' MB/s';
}
// 注：formatSize / formatTime 已统一到 ui.js 共享层（与读取页同一实现）

// === icns 预览：上传前在本地解析容器 ===
//
// icns 是 Apple 的图标容器，浏览器**无法直接显示**（即便 Chrome 把它标成 image/icns，
// 塞进 <img> 也是空白）。读取页的做法是走服务端 /api/thumb（后端 internal/app/icns.go
// 解析容器转 PNG），但上传页此刻文件还没上传、够不到服务端，只能在前端自己解析。
//
// 容器结构：8 字节头（"icns" + 大端总长）+ 若干块，每块 = 4 字节类型 + 4 字节长度
// （大端，含这 8 字节头）+ 数据。现代 icns 内嵌 PNG/JPEG，
// 老格式（ic04 / ic05 等 JP2、ARGB 原始像素）浏览器解不了，直接跳过。

// 内嵌图像的块类型，与后端 isIcnsImageChunk() 保持同一份清单。
const ICNS_IMAGE_TYPES = [
  'icp4', 'icp5', 'icp6', 'icp7', 'icp8', 'icp9', 'icpA',
  'ic07', 'ic08', 'ic09', 'ic10', 'ic11', 'ic12', 'ic13', 'ic14'
];
const ICNS_MAX_SIZE = 20 * 1024 * 1024; // 与后端 icnsMaxSize 一致：畸形文件不至于吃满内存
const ICNS_MAX_CHUNKS = 4096;           // 同理，防止伪造的块数量让循环空转
// 缩略图只有 48px（Retina 下 96px 物理像素），取短边 ≥128 的最小一块即可，
// 不必去解 1024px 那块（白费解码时间，且 iOS Safari 上大图解码容易失败）。
const ICNS_WANT_SIDE = 128;

// 按扩展名判断而非 MIME：各浏览器给 .icns 的 type 并不一致
// （Chrome 是 image/icns，Safari 可能是空串或 application/octet-stream），扩展名才可靠。
function isIcnsFile(file) {
  return /\.icns$/i.test((file && file.name) || '');
}

function readArrayBuffer(file) {
  // 用 FileReader 而不是 file.arrayBuffer()：后者 iOS 13 及更早不支持，
  // 而这个页面主要在手机浏览器上用。
  return new Promise((resolve, reject) => {
    const r = new FileReader();
    r.onload = () => resolve(r.result);
    r.onerror = () => reject(r.error);
    r.readAsArrayBuffer(file);
  });
}

// pngSide 返回该块的短边像素数；不是 PNG（或长度不足）返回 0。
// PNG 布局：8B 签名 + 4B 长度 + "IHDR" + 4B 宽 + 4B 高。
function pngSide(bytes) {
  if (bytes.length < 24) return 0;
  const dv = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  // 偏移：0=签名，8=IHDR 块长度(13)，12="IHDR"，16=宽，20=高
  if (dv.getUint32(0) !== 0x89504e47 || dv.getUint32(12) !== 0x49484452) return 0;
  return Math.min(dv.getUint32(16), dv.getUint32(20));
}

function isJpegBytes(bytes) {
  return bytes.length > 3 && bytes[0] === 0xff && bytes[1] === 0xd8 && bytes[2] === 0xff;
}

// icnsThumbURL 解析 icns 文件并返回可直接给 <img src> 的 blob URL。
// 解析失败、或容器里没有能解码的图像块时返回 null —— 调用方应回退成扩展名占位图标。
async function icnsThumbURL(file) {
  if (!file || file.size > ICNS_MAX_SIZE || file.size < 16) return null;

  let buf;
  try {
    buf = new Uint8Array(await readArrayBuffer(file));
  } catch (e) {
    return null; // 读取失败（权限/文件被移走）：不阻断上传，只放弃预览
  }
  if (buf.length < 16) return null;
  if (String.fromCharCode(buf[0], buf[1], buf[2], buf[3]) !== 'icns') return null;

  let best = null;      // 够用（短边 ≥ 目标）里最小的一块
  let bestSide = Infinity;
  let fallback = null;  // 都不够目标时的兜底：最大的一块
  let fallbackSide = 0;

  let off = 8; // 跳过 magic + 总长度
  for (let n = 0; off + 8 <= buf.length && n < ICNS_MAX_CHUNKS; n++) {
    // 长度字段是大端 uint32；用 DataView 读，避免手写位移在极端值下出错
    const size = new DataView(buf.buffer, buf.byteOffset + off + 4, 4).getUint32(0);
    if (size < 8 || off + size > buf.length) break; // 长度非法或文件被截断：停止，用已找到的
    const typ = String.fromCharCode(buf[off], buf[off + 1], buf[off + 2], buf[off + 3]);
    if (ICNS_IMAGE_TYPES.indexOf(typ) >= 0) {
      const body = buf.subarray(off + 8, off + size);
      const side = pngSide(body);
      if (side > 0) {
        if (side >= ICNS_WANT_SIDE && side < bestSide) { best = body; bestSide = side; }
        if (side > fallbackSide) { fallback = body; fallbackSide = side; }
      } else if (isJpegBytes(body) && !fallback) {
        fallback = body; // JPEG 读不出尺寸，仅在没有 PNG 块时兜底
      }
    }
    off += size;
  }

  const pick = best || fallback;
  if (!pick) return null;
  // slice() 复制一份：Blob 若直接持有整个文件的 buffer，会白占一份内存
  return URL.createObjectURL(new Blob([pick.slice()], {
    type: pngSide(pick) > 0 ? 'image/png' : 'image/jpeg'
  }));
}

// 生成本地预览用的 object URL。
// 显式补 MIME 的原因：部分系统 / 浏览器给不出 File.type（如 Windows 上的 .heic、
// 某些客户端里的 .mkv），空类型的 blob 浏览器不会渲染。这与读取页共用一份
// EXT_MIME（见 ui.js），两边对同一格式的判定才一致。
function previewURL(f) {
  const want = f.type || EXT_MIME[extOf(f.name)] || '';
  const src = (want && want !== f.type) ? new Blob([f], { type: want }) : f;
  return URL.createObjectURL(src);
}

// 给预览元素挂上「加载失败 → 回退扩展名占位块」：浏览器解不了的格式
// （如 Chrome 下的 TIFF、不支持的 HEIC / MKV）不留一个空白框。
function withThumbFallback(el, url, name) {
  el.onerror = () => { el.replaceWith(makeExtThumb(name)); URL.revokeObjectURL(url); };
  return el;
}

// makeExtThumb 生成「扩展名占位块」（非图片/视频，或 icns 解析失败时的兜底）。
function makeExtThumb(name) {
  const ph = document.createElement('div');
  ph.className = 'thumb-other';
  // 取文件后缀名（大写）作为图标；无后缀名时才回退到通用文件 SVG 图标
  const ext = (name.indexOf('.') >= 0) ? name.split('.').pop().toUpperCase() : '';
  if (ext) {
    ph.classList.add('thumb-ext');
    ph.textContent = ext.slice(0, 4);
  } else {
    ph.innerHTML = '<svg class="icon" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline></svg>';
  }
  return ph;
}

function getFiles(input) {
  if (!input.files.length) return;
  $('device').value = $('device').value.trim();
  upload(Array.from(input.files), input);
  input.value = '';
}

// 取文件在上传时的“逻辑名”：优先用拖文件夹时手动记录的相对路径，
// 其次用 <input webkitdirectory> 的 webkitRelativePath，最后退回纯文件名。
function fileUploadName(f) {
  return f.relPath || f.webkitRelativePath || f.name;
}

// 拍照 / 录像：一律交给系统原生输入，不再使用网页内相机（getUserMedia + MediaRecorder）。
//
// 为什么废弃网页相机：它依赖 getUserMedia 与 MediaRecorder，而飞牛手机客户端等 WebView
// 表现极不稳定——录像额外申请麦克风常被拒导致整体失败、MediaRecorder 可能残缺导致录完
// 拿不到文件，同一份代码在 Safari / Chrome / 各家客户端里行为也不一致。
//
// capture 一律保留（HTML 静态声明，见 upload.html）→ 一步直接调起原生相机 / 摄像机。
// 手机浏览器不看协议：HTTP 局域网直连与 HTTPS（用户自配 DDNS / 反代远程访问）下
// capture 都有效，无需按协议做任何分支；桌面浏览器本就忽略 capture，自然退化为
// 选择本地文件，这是浏览器行为。
//
// 关键：capture 只在 HTML 里【静态】声明，JS 运行时【不碰】——部分 WebView（飞牛 App）
// 只认「从未被 JS 写过」的静态属性：即使 setAttribute 的值与静态声明完全相同，
// 只要被 JS 写过一次（setAttribute / removeAttribute 都算），该 WebView 就会忽略
// capture（表现与没声明一样，点了没反应）。
function openCamera(mode) {
  const input = mode === 'video' ? $('recInput') : $('camInput');
  input.click();
}

['fileInput','folderInput','fileInputPhoto','camInput','recInput'].forEach(id => {
  $(id).addEventListener('change', () => getFiles($(id)));
});

// 拖放上传：主卡片拖放区支持拖入文件 / 文件夹；整块虚线区均可点击选文件
(function () {
  const dropArea = document.getElementById('dropArea');
  const uploadZone = document.getElementById('uploadZone');
  // 整个上传区（虚线框）点击即触发普通文件选择，不必只点“点击选择”文字
  dropArea.addEventListener('click', () => {
    const fi = document.getElementById('fileInput');
    if (fi) fi.click();
  });
  ['dragenter','dragover'].forEach(evt => {
    dropArea.addEventListener(evt, e => {
      e.preventDefault(); e.stopPropagation();
      uploadZone.classList.add('upload-zone--active');
    }, false);
  });
  ['dragleave','drop'].forEach(evt => {
    dropArea.addEventListener(evt, e => {
      e.preventDefault(); e.stopPropagation();
      uploadZone.classList.remove('upload-zone--active');
    }, false);
  });
  dropArea.addEventListener('drop', e => {
    e.preventDefault(); e.stopPropagation();
    uploadZone.classList.remove('upload-zone--active');

    // 优先用 File System Access 拖放 API 递归展开文件夹（拖入文件夹可上传其全部内容）
    const items = e.dataTransfer.items;
    if (items && items.length && items[0].webkitGetAsEntry) {
      const entries = [];
      for (const it of items) {
        const getAsEntry = it.webkitGetAsEntry || it.mozGetAsEntry;
        const entry = getAsEntry ? getAsEntry.call(it) : null;
        if (entry) entries.push(entry);
      }
      if (entries.length) {
        collectDroppedFiles(entries, files => {
          if (files.length) upload(files, null);
        });
        return;
      }
    }

    // 回退：普通多文件（不支持 entry API 的环境）
    const files = [];
    if (items) {
      for (const item of items) {
        if (item.kind === 'file') {
          const f = item.getAsFile();
          if (f) files.push(f);
        }
      }
    } else {
      for (const f of e.dataTransfer.files) files.push(f);
    }
    if (files.length) upload(files, null);
  }, false);
})();

// 拖入文件夹时，用 webkitGetAsEntry 递归收集目录内所有真实文件（目录本身不上传）
function traverseEntry(entry, basePath, out, done) {
  const full = basePath ? basePath + '/' + entry.name : entry.name;
  if (entry.isFile) {
    entry.file(f => { try { f.relPath = full; } catch (e) {} out.push(f); done(); }, done);
  } else if (entry.isDirectory) {
    const reader = entry.createReader();
    const readBatch = () => {
      reader.readEntries(batch => {
        if (!batch.length) { done(); return; }
        let pending = batch.length;
        batch.forEach(b => traverseEntry(b, full, out, () => {
          pending--;
          if (pending === 0) readBatch();
        }));
      }, done);
    };
    readBatch();
  } else {
    done();
  }
}

function collectDroppedFiles(entries, done) {
  const out = [];
  let pending = entries.length;
  if (pending === 0) { done(out); return; }
  entries.forEach(entry => traverseEntry(entry, '', out, () => {
    pending--;
    if (pending === 0) done(out);
  }));
}

function upload(files, inputEl) {
  if (!files || !files.length) return;
  const fd = new FormData();
  // 注意：multipart 的 filename 会被服务端/库清洗成纯文件名(丢掉目录)，
  // 所以相对路径(含层级)必须放在独立的 rels 字段，与 files 索引一一对应。
  files.forEach(f => {
    fd.append('files', f);
    fd.append('rels', fileUploadName(f));
  });
  var deviceName = $('device').value.trim();
  if (deviceName) rememberDeviceName(deviceName);
  fd.append('device', deviceName || 'default');

  const xhr = new XMLHttpRequest();
  const prog = $('progress');
  prog.style.display = 'block';
  prog.innerHTML = '';
  $('result').style.display = 'none';

  // 每个文件一行进度卡片（结构：上排缩略图/文件名/速度 + 取消按钮，下排独立进度条）
  const rows = files.map(f => {
    const item = document.createElement('div');
    item.className = 'up-prog-item';

    const row = document.createElement('div');
    row.className = 'up-prog-row';

    const main = document.createElement('div');
    main.className = 'file-main';

    // 左侧缩略图 / 文件图标
    // icns 单独走一条分支：它虽被判为 image/*，浏览器却渲染不出来，需先解析容器取内嵌图像
    if (isIcnsFile(f)) {
      const img = document.createElement('img');
      img.className = 'thumb';
      img.alt = '预览';
      main.appendChild(img);
      icnsThumbURL(f).then(url => {
        // 解析不出来（老格式 JP2/ARGB 容器）：退回扩展名占位块，不要留一个空白的 img
        if (!url) { img.replaceWith(makeExtThumb(f.name)); return; }
        img.onload = () => { img.classList.add('loaded'); URL.revokeObjectURL(url); };
        img.onerror = () => { img.replaceWith(makeExtThumb(f.name)); URL.revokeObjectURL(url); };
        img.src = url;
      });
    } else if (isPreviewImage(f)) {
      const img = document.createElement('img');
      img.className = 'thumb';
      img.alt = '预览';
      // 用 object URL 而不是 FileReader 的 data URL：后者会把整张图转 base64，
      // 体积再涨 1/3，大图在手机上很吃内存。
      const url = previewURL(f);
      img.onload = () => { img.classList.add('loaded'); URL.revokeObjectURL(url); };
      withThumbFallback(img, url, f.name);
      img.src = url;
      main.appendChild(img);
    } else if (isPreviewVideo(f)) {
      const v = document.createElement('video');
      v.className = 'thumb-video';
      v.muted = true; v.preload = 'metadata';
      // 视频不 revoke：src 还要继续用（播放），交由页面卸载时统一回收
      v.src = previewURL(f);
      withThumbFallback(v, v.src, f.name);
      main.appendChild(v);
    } else {
      main.appendChild(makeExtThumb(f.name));
    }

    const metaCol = document.createElement('div');
    metaCol.className = 'file-meta-col';
    const fname = document.createElement('div');
    fname.className = 'fname';
    fname.textContent = f.name;
    const fmeta = document.createElement('div');
    fmeta.className = 'fmeta';
    fmeta.textContent = '准备中…';
    metaCol.appendChild(fname);
    metaCol.appendChild(fmeta);
    main.appendChild(metaCol);

    // 右侧取消按钮
    const cancelBtn = document.createElement('button');
    cancelBtn.type = 'button';
    cancelBtn.className = 'up-prog-cancel';
    cancelBtn.setAttribute('aria-label', '取消上传');
    cancelBtn.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>';

    row.appendChild(main);
    row.appendChild(cancelBtn);

    // 下排：独立进度条
    const bar = document.createElement('div');
    bar.className = 'up-prog-bar';
    const fill = document.createElement('div');
    fill.className = 'up-prog-bar-fill';
    bar.appendChild(fill);

    item.appendChild(row);
    item.appendChild(bar);
    prog.appendChild(item);
    return { fmeta, fill, item, cancelBtn, f };
  });

  // 取消上传：单 xhr 整批共享，点任意行的叉号 → 整批 abort，所有行变 cancelled 状态后淡出
  const doCancel = () => {
    if (xhr.readyState !== 0 && xhr.readyState !== 4) {
      try { xhr.abort(); } catch (e) {}
    }
    rows.forEach(r => {
      r.item.classList.add('is-cancelled');
      r.cancelBtn.disabled = true;
      r.fmeta.textContent = '已取消';
    });
  };
  rows.forEach(r => { r.cancelBtn.onclick = doCancel; });

  // 速度计算：记录起始时间与已上报字节
  const t0 = performance.now();
  let lastLoaded = 0, lastT = t0;

  xhr.upload.onprogress = e => {
    if (!e.lengthComputable) return;
    const now = performance.now();
    const pct = (e.loaded / e.total * 100);
    // 整体瞬时速度（基于最近一次采样间隔）
    const dt = (now - lastT) / 1000;
    let speed = '';
    if (dt > 0.2) {
      const bps = (e.loaded - lastLoaded) / dt;
      speed = formatSpeed(bps);
      lastLoaded = e.loaded; lastT = now;
    }
    rows.forEach(r => {
      const p = Math.max(0, Math.min(100, pct));
      r.fmeta.textContent = (speed ? speed + ' · ' : '') + p.toFixed(0) + '%';
      r.fill.style.width = p + '%';
    });
  };
  xhr.onload = () => {
    if (xhr.status === 200) {
      const res = JSON.parse(xhr.responseText);
      prog.style.display = 'none';
      showResult(res, files);
    } else {
      prog.style.display = 'none';
      const box = $('result');
      box.style.display = 'block';
      let msg = '上传失败: ' + xhr.statusText;
      try {
        const res = JSON.parse(xhr.responseText);
        if (res && res.message) msg = '上传失败：' + res.message;
      } catch (e) {}
      box.innerHTML = '';
      const al = document.createElement('div'); al.className = 'alert alert-destructive';
      const at = document.createElement('div'); at.className = 'alert-title'; at.textContent = '上传失败';
      const ad = document.createElement('div'); ad.className = 'alert-desc'; ad.textContent = msg;
      al.appendChild(at); al.appendChild(ad); box.appendChild(al);
    }
  };
  xhr.onerror = () => {
    if (xhr.readyState === 4 && xhr.status === 0) {
      // 主动 abort 触发：不弹错误，由 doCancel 的 cancelled 状态接管
      return;
    }
    prog.style.display = 'none';
    const box = $('result');
    box.style.display = 'block';
    box.innerHTML = '';
    const al = document.createElement('div'); al.className = 'alert alert-destructive';
    const at = document.createElement('div'); at.className = 'alert-title'; at.textContent = '网络错误';
    const ad = document.createElement('div'); ad.className = 'alert-desc'; ad.textContent = '上传失败，请检查网络连接后重试。';
    al.appendChild(at); al.appendChild(ad); box.appendChild(al);
  };
  xhr.open('POST', 'upload');
  // 状态变更接口要求的客户端标识头：跨站请求无法携带（浏览器会先发预检，本服务不返回
  // CORS 许可 → 预检失败 → 请求发不出去），服务端据此拒绝跨站上传（防 CSRF）。
  xhr.setRequestHeader('X-Aellus-Client', '1');
  var deviceID = (typeof getDeviceID === 'function') ? getDeviceID() : '';
  if (deviceID) xhr.setRequestHeader('Deviceid', deviceID);
  xhr.send(fd);
  if (inputEl) inputEl.value = '';
}

function showResult(res, files) {
  const box = $('result');
  box.style.display = 'block';
  box.innerHTML = '';

  const head = document.createElement('div');
  head.className = 'result-head';
  head.textContent = '已保存到：' + res.dir;
  box.appendChild(head);

  const grid = document.createElement('div');
  grid.className = 'up-grid';
  res.files.forEach((f, i) => {
    const card = document.createElement('div');
    card.className = 'up-card';

    // 显示名走 ui.js 的 displayName()：普通文件落盘时会被服务端加上时间戳前缀
    //（20060102_150405.000000_，见 Go 端 resolveUploadTarget 防重名覆盖），这里去掉，
    // 与读取页卡片显示的名字完全一致。f.name（磁盘原名）不再直接展示。
    const shownName = displayName(f.name);
    const nameEl = document.createElement('div');
    nameEl.className = 'up-name';
    nameEl.title = shownName;
    nameEl.textContent = shownName;

    const thumb = document.createElement('div');
    thumb.className = 'up-thumb';
    if (files[i] && isIcnsFile(files[i])) {
      const img = document.createElement('img');
      img.alt = '预览';
      thumb.appendChild(img);
      icnsThumbURL(files[i]).then(url => {
        if (!url) { img.replaceWith(makeExtThumb(f.name)); return; }
        img.onload = () => URL.revokeObjectURL(url);
        img.onerror = () => { img.replaceWith(makeExtThumb(f.name)); URL.revokeObjectURL(url); };
        img.src = url;
      });
    } else if (files[i] && isPreviewImage(files[i])) {
      const img = document.createElement('img');
      img.alt = '预览';
      const url = previewURL(files[i]);
      withThumbFallback(img, url, f.name);
      img.src = url;
      thumb.appendChild(img);
    } else if (files[i] && isPreviewVideo(files[i])) {
      const v = document.createElement('video');
      v.className = 'thumb-video';
      v.muted = true; v.preload = 'metadata';
      v.src = previewURL(files[i]);
      withThumbFallback(v, v.src, f.name);
      thumb.appendChild(v);
    } else {
      const ph = document.createElement('div');
      ph.className = 'up-thumb-ph';
      ph.textContent = (f.name.split('.').pop() || 'file').toUpperCase().slice(0, 4);
      thumb.appendChild(ph);
    }

    const sizeEl = document.createElement('div');
    sizeEl.className = 'up-size';
    const time = f.mtime ? formatTime(f.mtime) : '';
    sizeEl.textContent = formatSize(f.size) + (time ? ' · ' + time : '');

    const meta = document.createElement('div');
    meta.className = 'up-meta';
    meta.appendChild(nameEl);
    meta.appendChild(sizeEl);

    card.appendChild(thumb);
    card.appendChild(meta);
    grid.appendChild(card);
  });
  box.appendChild(grid);
}

// 打开页面时恢复上次的设备名称
restoreDeviceName();

// 设备名清空即视为“清除记忆”：删掉本机记录，刷新后不再自动填充
(function bindDeviceClear() {
  var el = $('device');
  if (!el) return;
  el.addEventListener('input', function () {
    if (!el.value.trim()) {
      try { localStorage.removeItem(DEVICE_NAME_KEY); } catch (e) {}
    }
  });
})();
