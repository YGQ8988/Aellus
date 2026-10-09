// 读取页逻辑：选目录 → 列文件 → 下载 / 预览
//（$ / formatSize / formatDay / formatTime 等通用工具来自 ui.js 共享层）
// 可预览格式清单来自 ui.js 共享层（上传页用同一份，避免两页对同一格式判断不一致）。
// icns 不在这两份清单里：它是容器，读取页走服务端 /api/thumb 转 PNG（见下方 renderFile）。

// 图标 SVG（跨平台渲染一致）
const SVG_FOLDER  = '<svg class="icon" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>';
const SVG_FILE    = '<svg class="icon" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/></svg>';
const SVG_DOWNLOAD = '<svg class="icon" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>';

let currentDir = '';
let allDirs = []; // 最近一次 /api/dirs 返回的目录列表，用于决定面包屑是否显示「目录」层级
const PAGE_SIZE = 30;   // 每页条数（列表不足 2 页时不显示分页器）
let filesPage = 1;      // 文件列表：当前页 / 总数（分页器与空态判定用）
let filesTotal = 0;
let dirsPage = 1;       // 文件夹列表：当前页 / 总数
let dirsTotal = 0;
let previewFiles = []; // [{name, previewUrl, ext}, ...] 当前目录可预览文件
let lbIndex = 0;       // 灯箱当前索引
// 是否有删除权限（含删目录 / 批量删）：服务端按「桌面端本机 / 飞牛门户内」算好，随列表响应
// 的顶层 canDelete 下发——同一请求内所有条目一致（见 Go 端 canManage）。渲染列表前更新；
// 前端判定仅用于按钮显隐，真正的强制在服务端。
let canManage = false;
// 单文件下载一律走「直接导航」（见 downloadUrl），在飞牛门户 / 客户端内也能触发
// 浏览器自己的下载管理器，不再需要按环境隐藏下载入口。
// 历史：曾按「经飞牛网关 / iframe 嵌入」隐藏下载，因为当时的 fetch+blob 方案在
// WebView 里没有下载管理器、点了没反应。改用直连后该限制失去意义，且 viaGateway
// 判据本身漏掉了「门户 iframe 走裸端口 + 路径前缀」这种入口（无 X-Trim-Userid），
// 会出现「按钮显示了却点不动」的错位。批量 ZIP 仍走 blob（POST 无法直连）。
let hideDownload = false;

// applyHideDownload 保留为空实现：hideDownload 恒为 false，各处调用点无需改动，
// 便于将来若某个环境确实不支持下载时，只改这一处即可恢复隐藏。
function applyHideDownload() {}

function show(view) {
  $('dirsView').classList.toggle('active', view === 'dirs');
  $('filesView').classList.toggle('active', view === 'files');
  // 切换视图时清空另一视图的勾选，避免跨视图污染批量操作
  clearChecks(view === 'dirs' ? 'filesList' : 'dirsList');
  updateSelectedCount();
  if (typeof syncTopBarBlur === 'function') syncTopBarBlur();
  if (typeof syncToTop === 'function') syncToTop();
}

// ---- 通用页码分页器（文件夹列表 / 文件列表共用）----
// 不足 2 页（total ≤ size）时不显示；onGo(page) 由调用方提供（一般：回顶部 + 拉取该页）。
function renderPager(el, total, page, size, onGo) {
  el.innerHTML = '';
  const pages = Math.ceil(total / size);
  if (pages <= 1) { el.hidden = true; return; }
  el.hidden = false;
  const info = document.createElement('span');
  info.className = 'pager-total';
  info.textContent = '共 ' + total + ' 个';
  el.appendChild(info);
  function btn(label, act, cls) {
    const b = document.createElement('button');
    b.type = 'button';
    b.className = 'pager-btn' + (cls ? ' ' + cls : '');
    b.textContent = label;
    if (typeof act === 'number') b.onclick = () => onGo(act);
    else b.disabled = true;
    el.appendChild(b);
  }
  btn('‹', page > 1 ? page - 1 : null);
  // 页码窗口：≤7 页全列；否则 1 … (当前±1) … 末页
  const seq = [];
  if (pages <= 7) {
    for (let i = 1; i <= pages; i++) seq.push(i);
  } else {
    seq.push(1);
    if (page > 3) seq.push('…');
    for (let i = Math.max(2, page - 1); i <= Math.min(pages - 1, page + 1); i++) seq.push(i);
    if (page < pages - 2) seq.push('…');
    seq.push(pages);
  }
  seq.forEach(it => {
    if (it === '…') {
      const s = document.createElement('span');
      s.className = 'pager-ellipsis';
      s.textContent = '…';
      el.appendChild(s);
    } else if (it === page) {
      btn(String(it), null, 'active');
    } else {
      btn(String(it), it);
    }
  });
  btn('›', page < pages ? page + 1 : null);
}

// ---- 加载文件夹列表（分页：每页 PAGE_SIZE 条，超过 1 页时列表下方出现页码分页器）----
async function loadDirs(autoEnter = true, page) {
  const pg = page || 1;
  $('dirsLoading').style.display = 'block';
  $('dirsList').innerHTML = '';
  $('dirsEmpty').style.display = 'none';
  $('dirsPager').hidden = true;
  try {
    const res = await fetch('api/dirs?page=' + pg + '&size=' + PAGE_SIZE);
    const data = await res.json();
    allDirs = data.dirs;
    dirsPage = data.page || 1;
    dirsTotal = data.total || 0;
    $('dirsLoading').style.display = 'none';
    if (!data.total) {
      $('dirsEmpty').style.display = 'block';
      return;
    }
    if (!data.dirs.length) {
      // 本页为空但总数不为 0（页码越界，如删除后）→ 回退到最后一页
      const last = Math.ceil(data.total / PAGE_SIZE);
      if (pg > last) { loadDirs(false, last); return; }
    }
    canManage = !!data.canDelete;
    $('dirsList').innerHTML = data.dirs.map(d => {
      const delable = canManage;
      return `
      <div class="file-item dir-card">
        <input type="checkbox" class="file-check checkbox" data-name="${escapeAttr(d.name)}" data-del="${delable}" onchange="updateSelectedCount()" onclick="event.stopPropagation()">
        <div class="file-right">
          <div class="file-main" onclick="selectDir(${jsLit(d.name)})" style="cursor:pointer">
            <div class="thumb-folder">${SVG_FOLDER}</div>
            <div class="file-meta-col">
              <div class="fname fname-row"><span class="ftext">${escapeHtml(d.name === '' ? '未命名设备' : d.name)}</span><span class="badge badge-outline">文件夹</span></div>
              <div class="fmeta">${formatSize(d.size)} · ${formatCount(d.count)} 个文件 · ${formatDay(d.mtime)}</div>
            </div>
          </div>
          ${delable ? '<div class="file-actions"><a class="del-btn" data-name="' + escapeAttr(d.name) + '" onclick="event.stopPropagation(); onDeleteDir(this)">删除</a></div>' : ''}
        </div>
      </div>`;
    }).join('');
    // 显示目录页批量操作栏，重置全选
    $('dirsBatchBar').style.display = 'flex';
    $('selectAllDirs').checked = false;
    // 无删除权限（局域网设备直连）时直接隐藏批量删除按钮，不显示置灰态
    $('btnDelDirs').style.display = data.canDelete ? '' : 'none';
    updateSelectedCount();
    // 页码分页器：不足 2 页自动隐藏
    renderPager($('dirsPager'), dirsTotal, dirsPage, PAGE_SIZE, p => { window.scrollTo(0, 0); loadDirs(false, p); });
    // 刷新/重入时：若 sessionStorage 里保存了当前目录路径（且等于某个根目录或它的子路径），
    // 直接恢复到该目录，避免多目录场景刷新后回到目录选择页。
    // 仅在「没有任何记忆」时才按原规则：单目录自动进入，多目录展示目录选择页
    //（满足「返回首页后再读取文件应进目录页」的预期，因为返回首页会 clearAellusDir）。
    const saved = sessionStorage.getItem('aellus_currentDir');
    let target = null;
    if (autoEnter) {
      if (saved !== null && saved !== '') {
        // 仅当记忆的目录在当前（可能已切换）保存目录下真实存在时才恢复；
        // 否则视为失效（例如刚改过保存路径），交由下方兜底逻辑重新判定。
        const match = data.dirs.find(d => saved === d.name || saved.startsWith(d.name + '/'));
        if (match) target = saved;
      }
      // 兜底：无记忆、或记忆已失效时，若当前保存目录下恰好只有一个目录，则直接进入它。
      //（用 total 判定：分页下 data.dirs 可能只是第一页）
      if (target === null && data.total === 1 && data.dirs.length === 1) {
        target = data.dirs[0].name;
      }
    }
    if (target !== null) selectDir(target);
  } catch (e) {
    $('dirsLoading').textContent = '加载失败: ' + e.message;
  }
}

// ---- 打开某个目录（支持子目录路径，如 "设备名/子目录"；与文件夹列表共用同一套页码分页器）----
async function openDir(path, page) {
  currentDir = path;
  const pg = page || 1;
  try { sessionStorage.setItem('aellus_currentDir', path); } catch (e) {}
  show('files');
  // 首次（列表为空）才显示完整 loading 并清空；切换目录时保留旧列表、
  // 用半透明提示正在切换，避免整片清空出现的白屏“加载感”
  const firstLoad = $('filesList').children.length === 0;
  if (firstLoad) {
    $('filesLoading').style.display = 'block';
    $('filesList').innerHTML = '';
  } else {
    $('filesList').classList.add('swapping');
  }
  $('filesEmpty').style.display = 'none';
  $('filesPager').hidden = true;
  try {
    const res = await fetch('api/files?dir=' + encodeURIComponent(path) + '&page=' + pg + '&size=' + PAGE_SIZE);
    const data = await res.json();
    $('filesLoading').style.display = 'none';
    $('filesList').classList.remove('swapping');
    // data.error 也做 HTML 转义：服务端当前只返回固定文案，但这里是 innerHTML sink，
    // 一旦将来把用户可控内容（如目录名）拼进错误信息就会变成 XSS。
    if (data.error) { $('filesList').innerHTML = '<div class="empty">' + escapeHtml(data.error) + '</div>'; buildBreadcrumb(); return; }
    filesPage = data.page || 1;
    filesTotal = data.total || 0;
    if (!data.total) { previewFiles = []; $('filesList').innerHTML = ''; $('filesEmpty').style.display = 'block'; buildBreadcrumb(); return; }
    if (!data.files.length) {
      // 本页为空但目录有内容（页码越界，如删除后）→ 回退到最后一页
      const last = Math.ceil(data.total / PAGE_SIZE);
      if (pg > last) { openDir(path, last); return; }
    }
    // 构建可预览文件列表（图片 + 视频），供灯箱左右切换（文件夹不进预览）
    previewFiles = [];
    data.files.forEach((f, i) => {
      if (f.isDir) return;
      const ext = f.name.split('.').pop().toLowerCase();
      if (!isPreviewExt(ext)) return;
      const u = 'api/download?dir=' + encodeURIComponent(path) + '&file=' + encodeURIComponent(f.name);
      // icns 是容器格式，浏览器不原生显示 → 预览走服务端转 PNG（thumb 接口大图），
      // 下载仍用原文件（downloadUrl），灯箱下载按钮据此取原文件。
      // svg 的 previewUrl 是占位：真实地址要等取回字节建好 blob 后填（见 hydrateSvgPreviews）。
      const isIcns = ext === 'icns';
      previewFiles.push({
        name: f.name,
        previewUrl: isIcns ? u.replace('api/download', 'api/thumb') + '&w=800' : u + '&inline=1',
        downloadUrl: u, ext: ext, idx: i, size: f.size || 0
      });
    });
    canManage = !!data.canDelete;
    // 上一页为 svg 建的 blob URL 必须回收，否则翻页/换目录会一路累积到内存里
    revokeSvgBlobs();
    $('filesList').innerHTML = data.files.map(renderFile).join('');
    hydrateSvgPreviews();   // 不 await：缩略图填充不该挡住列表渲染与分页器
    // 显示批量操作栏，重置选中状态
    $('batchBar').style.display = 'flex';
    $('selectAll').checked = false;
    // 无删除权限（局域网设备直连）时直接隐藏批量删除按钮，不显示置灰态
    $('btnDelSelected').style.display = data.canDelete ? '' : 'none';
    updateSelectedCount();
    buildBreadcrumb();
    // 页码分页器：不足 2 页自动隐藏
    renderPager($('filesPager'), filesTotal, filesPage, PAGE_SIZE, p => { window.scrollTo(0, 0); openDir(currentDir, p); });
  } catch (e) {
    $('filesLoading').style.display = 'none';
    $('filesList').classList.remove('swapping');
    $('filesList').innerHTML = '<div class="empty">加载失败: ' + escapeHtml(e.message) + '</div>';
    buildBreadcrumb();
  }
}

// 进入某个目录（设备根目录或子目录路径）
function selectDir(name) { openDir(name); }
// 在当前目录下进入子文件夹
function enterFolder(name) { openDir(currentDir ? currentDir + '/' + name : name); }
// 返回首页时清掉“上次目录”记忆，使下次从首页进读取文件时走目录选择页
function clearAellusDir() { try { sessionStorage.removeItem('aellus_currentDir'); } catch (e) {} }

// 根据 currentDir（可能是多层路径）动态生成面包屑
function buildBreadcrumb() {
  // 返回首页必须用相对路径 "./"：它会按页面 <base> 解析——
  //   飞牛门户内（base=/app/<appname>/）→ 回应用首页；
  //   局域网直连（base=/）→ 回站点首页。
  // 写死 "/" 会跑到站点根：门户内即飞牛桌面（iframe 跳出应用），局域网下也会丢应用前缀。
  const parts = ['<a class="bc-link" href="./" onclick="clearAellusDir()">← 返回首页</a>'];
  if (allDirs.length > 1) {
    parts.push('<span class="bc-sep">/</span>');
    // 不用 href="javascript:..."：它依赖 CSP 的 'unsafe-inline'，一旦收紧 CSP 就静默失效。
    parts.push('<a class="bc-link bc-dirs" href="#" onclick="backToDirs(); return false;">目录</a>');
  }
  const segs = (currentDir || '').split('/').filter(s => s !== '');
  if (segs.length === 0) {
    parts.push('<span class="bc-sep">/</span>');
    parts.push('<span class="bc-cur">' + (currentDir === '' ? '未命名设备' : escapeHtml(currentDir)) + '</span>');
  } else {
    let acc = '';
    segs.forEach((s, i) => {
      acc = acc ? acc + '/' + s : s;
      parts.push('<span class="bc-sep">/</span>');
      if (i === segs.length - 1) {
        parts.push('<span class="bc-cur">' + escapeHtml(s) + '</span>');
      } else {
        // 用 data-nav 存编码后的路径，onclick 再解码，避免 JSON.stringify 产生的双引号
        // 与外层 href="..." 双引号冲突导致链接失效（无法返回上一级）。
        parts.push('<a class="bc-link" href="#" data-nav="' + encodeURIComponent(acc) + '" onclick="selectDir(decodeURIComponent(this.dataset.nav)); return false;">' + escapeHtml(s) + '</a>');
      }
    });
  }
  $('breadcrumb').innerHTML = parts.join('');
}

// 是否可预览（图片或视频）。icns 需单独放行：它不在共享清单里，
// 但读取页有服务端转 PNG 这条路径，所以同样算可预览。
function isPreviewExt(ext) {
  return PREVIEW_IMG_EXTS.includes(ext) || PREVIEW_VID_EXTS.includes(ext) || ext === 'icns';
}
function isVideoExt(ext) { return PREVIEW_VID_EXTS.includes(ext); }

function renderFile(f, idx) {
  // 文件夹：与文件卡片结构一致（复选框、缩略图、文件名、标签、删除）。
  // 注意：文件夹卡不再提供「打开」按钮（点击缩略图/文件名区即可进入）。
  if (f.isDir) {
    const delable = canManage;
    return `
      <div class="file-item folder-item">
        <input type="checkbox" class="file-check checkbox" data-name="${escapeAttr(f.name)}" data-del="${delable}" onchange="updateSelectedCount()" onclick="event.stopPropagation()">
        <div class="file-right">
          <div class="file-main" onclick="enterFolder(${jsLit(f.name)})" style="cursor:pointer">
            <div class="thumb-folder">${SVG_FOLDER}</div>
            <div class="file-meta-col">
              <div class="fname fname-row"><span class="ftext">${escapeHtml(f.name)}</span><span class="badge badge-outline">文件夹</span></div>
              <div class="fmeta">${formatSize(f.size)} · ${formatCount(f.count)} 个文件 · ${formatDay(f.mtime)}</div>
            </div>
          </div>
          ${delable ? '<div class="file-actions"><a class="del-btn" data-name="' + escapeAttr(f.name) + '" onclick="event.stopPropagation(); onDelete(this)">删除</a></div>' : ''}
        </div>
      </div>`;
  }
  const url = 'api/download?dir=' + encodeURIComponent(currentDir) + '&file=' + encodeURIComponent(f.name);
  const meta = formatSize(f.size) + ' · ' + formatTime(f.mtime);
  const ext = f.name.split('.').pop().toLowerCase();
  const isImg = isPreviewExt(ext) && !isVideoExt(ext);
  const isVid = isVideoExt(ext);
  const previewable = isImg || isVid;
  let thumb;
  if (previewable) {
    if (isImg) {
      const nameAttr = `data-name="${escapeAttr(f.name)}"`;
      if (ext === 'svg') {
        // svg 不能直接用 /api/thumb：服务端对可携带脚本的类型一律不内联（防存储型 XSS），
        // 返回的是 application/octet-stream，<img> 拿到也渲染不出来。
        // 改成取回字节后以 image/svg+xml 建 blob 再交给 <img>（见 hydrateSvgPreviews）：
        // SVG 经 <img> 渲染时浏览器走「安全静态模式」，不执行脚本、不加载外部引用，
        // 与上传页「本地 blob + <img>」是同一条安全路径。
        // 这里先不设 src（等 blob 就绪），先占好位置避免列表抖动。
        thumb = `<img class="thumb" alt="" decoding="async" ${nameAttr} data-idx="${idx}" data-svg="1" style="cursor:pointer" onclick="event.stopPropagation(); openLightboxFromEl(this)">`;
      } else {
        // 缩略图走 /api/thumb（服务端缩放），只拉几百字节的小图，避免整张原图卡顿
        const thumbUrl = 'api/thumb?dir=' + encodeURIComponent(currentDir) + '&file=' + encodeURIComponent(f.name) + '&w=240';
        thumb = `<img class="thumb" src="${thumbUrl}" alt="" loading="lazy" decoding="async" onload="this.classList.add('loaded')" onerror="onThumbError(this)" ${nameAttr} style="cursor:pointer" onclick="event.stopPropagation(); openLightboxFromEl(this)">`;
      }
    } else {
      thumb = `<video class="thumb-video" src="${url}" preload="metadata" onerror="onThumbError(this)" data-name="${escapeAttr(f.name)}" style="cursor:pointer" onclick="event.stopPropagation(); openLightboxFromEl(this)"></video>`;
    }
  } else {
    // 其他文件：与上传页已上传列表一致——居中大号扩展名文字；
    // 无扩展名 / 隐藏文件不显示文字，退回通用文件图标
    let extText = '';
    if (ext && f.name.indexOf('.') > 0 && !f.name.startsWith('.')) {
      extText = ext.toUpperCase().slice(0, 4);
    }
    thumb = extText
      ? `<div class="thumb-other thumb-ext">${escapeHtml(extText)}</div>`
      : `<div class="thumb-other">${SVG_FILE}</div>`;
  }
  const mainCursor = previewable ? ' style="cursor:pointer"' : '';
  const mainClick = previewable ? ` data-name="${escapeAttr(f.name)}" onclick="openLightboxFromEl(this)"` : '';
  const delable = canManage;
  return `
    <div class="file-item">
      <input type="checkbox" class="file-check checkbox" data-name="${escapeAttr(f.name)}" data-del="${delable}" onchange="updateSelectedCount()" onclick="event.stopPropagation()">
      <div class="file-right">
        <div class="file-main"${mainClick}${mainCursor}>
          ${thumb}
          <div class="file-meta-col">
            <div class="fname">${escapeHtml(displayName(f.name))}</div>
            <div class="fmeta">${meta}</div>
          </div>
        </div>
        <div class="file-actions">
          ${hideDownload ? '' : `<a class="dl-btn" data-url="${url}" data-name="${escapeAttr(f.name)}" onclick="event.stopPropagation(); onSingleDownload(this)">下载</a>`}
          <a class="share-btn" data-url="${url}" data-name="${escapeAttr(f.name)}" onclick="event.stopPropagation(); onShare(this)">分享</a>
          ${delable ? '<a class="del-btn" data-name="' + escapeAttr(f.name) + '" onclick="event.stopPropagation(); onDelete(this)">删除</a>' : ''}
        </div>
      </div>
    </div>
  `;
}

// ---- svg 预览（读取页） ----
//
// 上传页渲染 svg 很容易：文件就在本地，拿 File 建个 blob / data URL 给 <img> 即可。
// 读取页不同——文件在服务端，而服务端对 svg 一律按 application/octet-stream +
// attachment 输出（见 Go 端 fileout.go：svg 可携带脚本，内联即存储型 XSS）。
// 直接把下载链接塞给 <img> 是渲染不出来的（还带 nosniff，浏览器也不会按内容猜）。
//
// 所以这里取回字节、以 image/svg+xml 重新建 blob 再交给 <img>。安全性与上传页一致：
// SVG 经 <img> 渲染时浏览器走「安全静态模式」——不执行脚本、不加载外部引用。
// 这条路径不需要改动服务端那条「可携带脚本的类型绝不内联」的规则。
const SVG_PREVIEW_MAX_BYTES = 8 * 1024 * 1024; // 超过则不预览：svg 是文本，可能有极大的
let svgBlobURLs = [];
let svgGen = 0;   // 列表代次：翻页 / 换目录后上一批异步结果作废

function revokeSvgBlobs() {
  svgGen++;
  svgBlobURLs.forEach(u => { try { URL.revokeObjectURL(u); } catch (e) {} });
  svgBlobURLs = [];
}

// 扩展名占位块（DOM 版）：与 renderFile 里非可预览文件的展示保持一致
function extThumbEl(name) {
  const ph = document.createElement('div');
  ph.className = 'thumb-other';
  const ext = extOf(name).toUpperCase().slice(0, 4);
  if (ext) { ph.classList.add('thumb-ext'); ph.textContent = ext; }
  else { ph.innerHTML = SVG_FILE; }
  return ph;
}

// 缩略图加载失败：浏览器解不了这个格式（如 Chrome 下的 TIFF、不支持的 HEIC）。
// 回退成扩展名占位块，同时把它从可预览列表里摘掉——否则点开灯箱只会看到一张空白图。
function onThumbError(el) {
  const name = el.dataset ? el.dataset.name : '';
  if (name) previewFiles = previewFiles.filter(p => p.name !== name);
  el.replaceWith(extThumbEl(name));
}

// 给列表里所有 svg 缩略图填上真实图像（renderFile 先把 <img> 占位，src 后补）
async function hydrateSvgPreviews() {
  const gen = svgGen;
  const imgs = Array.prototype.slice.call(document.querySelectorAll('#filesList img.thumb[data-svg]'));
  if (!imgs.length) return;
  const failed = [];
  await Promise.all(imgs.map(async img => {
    const entry = previewFiles.filter(p => String(p.idx) === String(img.dataset.idx))[0];
    if (!entry) return;
    const drop = () => { failed.push(entry); if (gen === svgGen) onThumbError(img); };
    if (entry.size && entry.size > SVG_PREVIEW_MAX_BYTES) { drop(); return; }
    try {
      const res = await fetch(entry.downloadUrl);
      if (!res.ok) throw new Error('HTTP ' + res.status);
      const blob = await res.blob();
      // 必须用 slice 显式改写类型：服务端给的是 application/octet-stream，
      // blob 沿用该类型的话 <img> 不会渲染。slice 只改类型、不复制字节。
      const u = URL.createObjectURL(blob.slice(0, blob.size, 'image/svg+xml'));
      if (gen !== svgGen) { URL.revokeObjectURL(u); return; }  // 已经翻页 / 换目录了
      svgBlobURLs.push(u);
      entry.previewUrl = u;
      img.onload = () => img.classList.add('loaded');
      img.onerror = () => onThumbError(img);
      img.src = u;
    } catch (e) {
      drop();
    }
  }));
  if (failed.length && gen === svgGen) {
    previewFiles = previewFiles.filter(p => failed.indexOf(p) < 0);
  }
}

// ---- 批量下载 ----
function toggleSelectAll(checked, listId) {
  document.querySelectorAll('#' + listId + ' .file-check').forEach(c => { c.checked = checked; });
  updateSelectedCount();
}
function clearChecks(listId) {
  document.querySelectorAll('#' + listId + ' .file-check').forEach(c => { c.checked = false; });
}
// 批量栏「取消」：清空当前列表的所有勾选并复位全选框
function clearAllChecks(listId, selectId) {
  clearChecks(listId);
  $(selectId).checked = false;
  updateSelectedCount();
}

// 展示名 displayName() 已上移到 ui.js 共享层（上传页结果卡与删除弹窗要用同一份规则），
// 这里不再重复定义：卡片、分享弹窗、删除确认一律走 displayName()，与读取页展示一致。
// 注意 data-name 上挂的仍是磁盘原名——下载 / 删除 / 缩略图请求必须用它。

function updateSelectedCount() {
  // 文件页：只统计文件列表内的勾选；批量删除仅统计勾选中可删的项
  const nFiles = document.querySelectorAll('#filesList .file-check:checked').length;
  const delFiles = Array.from(document.querySelectorAll('#filesList .file-check:checked')).filter(c => c.dataset.del === 'true').length;
  $('btnSelected').disabled = nFiles === 0;
  $('btnDelSelected').disabled = delFiles === 0;
  const btnCancelFiles = document.getElementById('btnCancelFiles');
  if (btnCancelFiles) btnCancelFiles.style.display = nFiles > 0 ? '' : 'none';
  // 目录页：只统计目录列表内的勾选；批量删除仅统计勾选中可删的目录
  const nDirs = document.querySelectorAll('#dirsList .file-check:checked').length;
  const delDirs = Array.from(document.querySelectorAll('#dirsList .file-check:checked')).filter(c => c.dataset.del === 'true').length;
  const btnDelDirs = document.getElementById('btnDelDirs');
  if (btnDelDirs) btnDelDirs.disabled = delDirs === 0;
  const btnDownloadDirs = document.getElementById('btnDownloadDirs');
  if (btnDownloadDirs) btnDownloadDirs.disabled = nDirs === 0;
  const btnCancelDirs = document.getElementById('btnCancelDirs');
  if (btnCancelDirs) btnCancelDirs.style.display = nDirs > 0 ? '' : 'none';
}

// 删除选中的文件：复制「下载选中」的思路，逐个调 /api/delete，成功后即时移除卡片。
// 仅删除可删的项，其余跳过并提示。
async function deleteSelected(btn) {
  const checked = Array.from(document.querySelectorAll('#filesList .file-check:checked'));
  const files = checked.filter(c => c.dataset.del === 'true').map(c => c.dataset.name);
  const skipped = checked.length - files.length;
  if (!files.length) return;
  const desc = '确定删除选中的 ' + files.length + ' 个文件？'
    + (skipped ? '另有 ' + skipped + ' 个来自其他设备、仅可下载，将跳过。' : '')
    + '此操作不可恢复。';
  const ok = await confirmDialog({ title: '删除选中的文件', desc: desc, confirmText: '删除', cancelText: '取消', destructive: true });
  if (!ok) return;
  const btns = document.querySelectorAll('.btn-batch');
  const states = Array.from(btns).map(b => ({ el: b, html: b.innerHTML }));
  btns.forEach(b => { b.disabled = true; });
  try {
    for (const name of files) {
      await apiDelete(currentDir, name);   // 失败已在 apiDelete 内 toast
    }
  } finally {
    btns.forEach(b => { b.classList.remove('loading'); b.innerHTML = states.find(s => s.el === b).html; });
    updateSelectedCount();
    // 分页下删除会让后续页错位，统一重拉当前页以反映实际状态；
    // 本页被删空时 openDir 会自动回退到最后一页。
    loadDirs(false);                       // 刷新文件夹列表的文件计数
    openDir(currentDir, filesPage);
  }
}

// 目录页「删除选中目录」：每个勾选项都是顶层目录，删除路径为 ('' , dirName)
// 仅删除可删的目录，其余跳过并提示。
async function deleteSelectedDirs(btn) {
  const checked = Array.from(document.querySelectorAll('#dirsList .file-check:checked'));
  const dirs = checked.filter(c => c.dataset.del === 'true').map(c => c.dataset.name);
  const skipped = checked.length - dirs.length;
  if (!dirs.length) return;
  const desc = '确定删除选中的 ' + dirs.length + ' 个目录及其全部内容？'
    + (skipped ? '另有 ' + skipped + ' 个来自其他设备、仅可下载，将跳过。' : '')
    + '此操作不可恢复。';
  const ok = await confirmDialog({ title: '删除选中的目录', desc: desc, confirmText: '删除', cancelText: '取消', destructive: true });
  if (!ok) return;
  const btns = Array.from(document.querySelectorAll('#dirsList .btn-batch'));
  btns.forEach(b => { b.disabled = true; });
  btn.classList.add('loading');
  btn.innerHTML = '<span class="spinner"></span>删除中...';
  try {
    for (const d of dirs) { await apiDelete('', d); }
  } catch (e) {
    toast('删除失败: ' + e.message, { variant: 'destructive' });
  } finally {
    loadDirs(false, dirsPage);   // 分页下重拉当前页（删空自动回退）
    updateSelectedCount();
  }
}

// 目录卡片单条「删除」：顶层目录，删除路径为 ('' , dirName)
async function onDeleteDir(btn) {
  const name = btn.dataset.name;
  if (btn.classList.contains('loading')) return;
  const ok = await confirmDialog({ title: '删除目录', desc: '确定删除「' + name + '」及其全部内容？此操作不可恢复。', confirmText: '删除', cancelText: '取消', destructive: true });
  if (!ok) return;
  btn.classList.add('loading');
  const old = btn.textContent;
  btn.textContent = '删除中';
  const delOk = await apiDelete('', name);
  if (!delOk) { btn.classList.remove('loading'); btn.textContent = old; return; }
  updateSelectedCount();
  loadDirs(false, dirsPage);   // 重拉当前页（分页下保证准确；删空自动回退）
}

async function downloadSelected(btn) {
  const files = Array.from(document.querySelectorAll('#filesList .file-check:checked')).map(c => c.dataset.name);
  if (!files.length) return;
  await downloadBatch(files, btn);
}

// 目录页批量下载：逐个选中目录打包成 ZIP（files 留空表示整目录），多目录则依次触发下载。
async function downloadSelectedDirs(btn) {
  const dirs = Array.from(document.querySelectorAll('#dirsList .file-check:checked')).map(c => c.dataset.name);
  if (!dirs.length) return;
  const btns = document.querySelectorAll('.btn-batch');
  btns.forEach(b => { b.disabled = true; });
  btn.classList.add('loading');
  btn.innerHTML = '<span class="spinner"></span>下载中...';
  try {
    for (const dir of dirs) {
      const blob = await fetchBlob('api/download-batch', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ dir: dir, files: [] }),
      });
      if (blob) downloadBlob(blob, (dir.split('/').pop() || 'dir') + '.zip');
    }
  } finally {
    btn.classList.remove('loading');
    btn.textContent = '下载';
    updateSelectedCount();
  }
}

// 单文件下载：先 HEAD 探活（确认文件可取、名字没错），再直接导航到下载地址。
// 不用 fetch+blob 的原因见 downloadUrl 注释；HEAD 探活让我们能在失败时弹 toast，
// 而不是把用户导航到一个服务端错误页。成功即返回，页面不离开（服务端是 attachment）。
async function onSingleDownload(btn) {
  if (btn.classList.contains('loading')) return;
  btn.classList.add('loading');
  btn.innerHTML = '<span class="spinner"></span>下载中';
  try {
    if (!(await probeDownload(btn.dataset.url))) return;
    downloadUrl(btn.dataset.url);
  } finally {
    btn.classList.remove('loading');
    btn.textContent = '下载';
  }
}

// downloadUrl 的前置探活：HEAD 请求只取响应头、不传文件体，成本极低。
// 返回 true 表示可以安全导航；失败时统一弹 toast 并返回 false。
async function probeDownload(url) {
  try {
    const res = await fetch(url, { method: 'HEAD' });
    if (res.ok) return true;
    toast('下载失败: ' + res.status, { variant: 'destructive' });
    return false;
  } catch (e) {
    toast('下载失败: ' + e.message, { variant: 'destructive' });
    return false;
  }
}

// ---- 分享：弹出文件下载链接二维码 ----
// 地址 = 当前 origin + <base> 前缀 + 相对下载路径（桌面端 /api/...，飞牛网关 /app/<name>/api/...），
// 手机扫码即可直接下载；弹窗复用首页「扫码访问」的 qr-modal 结构（样式差异在 browse.css）。
async function onShare(btn) {
  const modal = $('shareModal');
  const box = $('shareQrCode');
  const nameEl = $('shareFileName');
  if (nameEl) nameEl.textContent = displayName(btn.dataset.name || '');
  const baseEl = document.querySelector('base');
  const baseHref = baseEl ? baseEl.getAttribute('href') : '/';
  // 二维码地址一律取 /api/addr 返回的「裸端口直连」地址，不能用 location.origin：
  //   - 本机常用 localhost 访问，扫码方根本访问不到 localhost；
  //   - 飞牛门户若用 location.origin，路径要过网关登录态，手机扫码后下载不了。
  // 裸端口是免登录的上传/下载入口，且同样接受门户前缀（/app/<name>/api/...，
  // 由 Go 端 withPrefix 归一化），所以门户内也照样可用——baseHref 照常拼在后面。
  let origin = location.origin;
  try {
    const res = await fetch('api/addr');
    const d = await res.json();
    if (d && d.url) origin = d.url;
  } catch (e) {}
  const url = origin + baseHref + btn.dataset.url;
  try {
    const qr = qrcode(0, 'L');
    qr.addData(url);
    qr.make();
    box.innerHTML = qr.createSvgTag(8, 4);
    const copyBtn = $('shareCopy');
    if (copyBtn) copyBtn.dataset.url = url;
  } catch (e) {
    box.innerHTML = '<p style="color:hsl(var(--muted-foreground));font-size:13px;margin:8px 0">二维码生成失败</p>';
  }
  if (window.lockScroll) lockScroll();
  modal.classList.add('active');
  document.addEventListener('keydown', shareKeyHandler);
}

function closeShare() {
  $('shareModal').classList.remove('active');
  if (window.unlockScroll) unlockScroll();
  document.removeEventListener('keydown', shareKeyHandler);
}

// 分享弹窗：复制文件下载链接（交互对齐首页 LAN 地址栏复制：点一下复制，图标切换为对勾并变绿）
function copyShareUrl() {
  const btn = $('shareCopy');
  const url = btn ? (btn.dataset.url || '') : '';
  function ok() {
    if (!btn) return;
    btn.classList.add('copied');
    setTimeout(() => { btn.classList.remove('copied'); }, 1200);
  }
  function fallback() {
    const ta = document.createElement('textarea');
    ta.value = url; ta.setAttribute('readonly', '');
    ta.style.position = 'fixed'; ta.style.top = '0'; ta.style.left = '0'; ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.focus(); ta.select();
    ta.setSelectionRange(0, ta.value.length);
    let done = false;
    try { done = document.execCommand('copy'); } catch (e) {}
    document.body.removeChild(ta);
    if (done) ok();
  }
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(url).then(ok).catch(fallback);
  } else {
    fallback();
  }
}

// 分享弹窗 ESC 关闭（与灯箱同一模式：打开时注册、关闭时移除，避免常驻监听互相干扰）
function shareKeyHandler(e) {
  if (e.key === 'Escape') {
    closeShare();
    // ESC 是键盘交互：关闭后触发按钮会残留 :focus-visible 描边，主动移除焦点消除
    if (document.activeElement && document.activeElement.blur) document.activeElement.blur();
  }
}

// 单文件删除：确认后调 /api/delete，成功后刷新文件列表（重建 previewFiles 与索引）
async function onDelete(btn) {
  const name = btn.dataset.name;
  if (btn.classList.contains('loading')) return;
  // 弹窗里显示展示名（去掉时间戳前缀），与卡片上的名字一致；删除请求仍用磁盘原名 name
  const ok = await confirmDialog({ title: '删除文件', desc: '确定删除「' + displayName(name) + '」？此操作不可恢复。', confirmText: '删除', cancelText: '取消', destructive: true });
  if (!ok) return;
  btn.classList.add('loading');
  const old = btn.textContent;
  btn.textContent = '删除中';
  const delOk = await apiDelete(currentDir, name);
  if (!delOk) {
    btn.classList.remove('loading');
    btn.textContent = old;
    return;
  }
  // 成功后重拉当前页（分页下保证数据准确；本页删空会自动回退到最后一页）
  loadDirs(false);                       // 刷新文件夹列表的文件计数
  openDir(currentDir, filesPage);
}

async function downloadBatch(files, triggerBtn) {
  const btns = document.querySelectorAll('.btn-batch');
  const states = Array.from(btns).map(b => ({ el: b, html: b.innerHTML }));
  btns.forEach(b => { b.disabled = true; });
  if (triggerBtn) {
    triggerBtn.classList.add('loading');
    triggerBtn.innerHTML = '<span class="spinner"></span>下载中...';
  }
  try {
    const blob = await fetchBlob('api/download-batch', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ dir: currentDir, files: files }),
    });
    if (blob) {
      const zipBase = currentDir.split('/').pop() || 'files';
      downloadBlob(blob, zipBase + '.zip');
    }
  } finally {
    btns.forEach(b => { b.classList.remove('loading'); b.innerHTML = states.find(s => s.el === b).html; });
    updateSelectedCount();
  }
}

// ---- 灯箱预览：左右切换（PC 按钮 / 移动端滑动）+ 键盘导航 ----
function openLightbox(idx) {
  if (idx < 0 || idx >= previewFiles.length) return;
  lbIndex = idx;
  showLbImage();
  $('lightbox').style.display = 'flex';
  lockScroll(); // 锁 html+body，背景不可滚动（html 是实际滚动容器，只锁 body 无效）
  const lb = $('lightbox');
  lb.addEventListener('touchstart', lbTouchStart, { passive: true });
  lb.addEventListener('touchmove', lbTouchMove, { passive: false });
  lb.addEventListener('touchend', lbTouchEnd, { passive: true });
  document.addEventListener('keydown', lbKeyHandler);
}

// 按文件名打开灯箱：点击时实时在 previewFiles 中查找索引，
// 避免删除文件后其余卡片仍持有失效的旧 idx（删除后 previewFiles 已被过滤、索引错位）。
function openLightboxFromEl(el) {
  const name = el.dataset.name;
  const i = previewFiles.findIndex(p => p.name === name);
  if (i >= 0) openLightbox(i);
}

function closeLightbox() {
  $('lightbox').style.display = 'none';
  $('lbVideo').pause();
  unlockScroll();
  const lb = $('lightbox');
  lb.removeEventListener('touchstart', lbTouchStart);
  lb.removeEventListener('touchmove', lbTouchMove);
  lb.removeEventListener('touchend', lbTouchEnd);
  document.removeEventListener('keydown', lbKeyHandler);
  lbTouchStartPt = null;
  // 清掉平移切换可能的中途残留（幽灵层 + 主图内联样式），保证下次打开是干净状态
  document.querySelectorAll('.lb-ghost').forEach(g => g.remove());
  const li = $('lbImg');
  li.style.transition = ''; li.style.transform = ''; li.style.opacity = '';
}

// ---- 灯箱触摸滑动切换（移动端；桌面端仍用左右箭头） ----
let lbTouchStartPt = null;
function lbTouchStart(e) {
  if (e.touches.length !== 1) return;
  // 点到按钮（关闭/下载/删除）不触发滑动
  if (e.target.closest && e.target.closest('button')) return;
  lbTouchStartPt = { x: e.touches[0].clientX, y: e.touches[0].clientY };
}
function lbTouchMove(e) {
  if (!lbTouchStartPt || e.touches.length !== 1) return;
  const dx = e.touches[0].clientX - lbTouchStartPt.x;
  const dy = e.touches[0].clientY - lbTouchStartPt.y;
  // 横向主导时阻止浏览器默认手势（下拉刷新/橡皮筋），视频控件区域除外（避免破坏进度条拖动）
  if (Math.abs(dx) > Math.abs(dy) && !(e.target.closest && e.target.closest('video'))) {
    e.preventDefault();
  }
}
function lbTouchEnd(e) {
  if (!lbTouchStartPt || e.changedTouches.length !== 1) return;
  const t = e.changedTouches[0];
  const dx = t.clientX - lbTouchStartPt.x;
  const dy = t.clientY - lbTouchStartPt.y;
  const startY = lbTouchStartPt.y;
  lbTouchStartPt = null;
  if (previewFiles.length < 2) return;              // 仅 1 个文件不切换
  if (Math.abs(dx) < 60 || Math.abs(dx) < Math.abs(dy)) return; // 位移不够或竖向手势
  // 视频：触摸起点在底部进度条区域（~72px）不触发，避免拖动进度条误切
  if ($('lbVideo').style.display !== 'none' && startY > window.innerHeight - 72) return;
  lbNav(dx < 0 ? 1 : -1);
}

function lbNav(delta) {
  lbIndex = (lbIndex + delta + previewFiles.length) % previewFiles.length;
  showLbImage(delta);
}

// 预加载相邻图片（仅图片），翻页时直接走浏览器缓存，消除加载闪烁
function preloadNeighbors() {
  if (previewFiles.length < 2) return;
  [lbIndex - 1, lbIndex + 1].forEach(i => {
    const p = previewFiles[(i + previewFiles.length) % previewFiles.length];
    if (p && !isVideoExt(p.ext)) {
      const im = new Image();
      im.src = p.previewUrl;
    }
  });
}

// dir：切换方向（+1 下一张 / -1 上一张；不传 = 无方向：首开、删除补位等直接淡入）。
// 前后都是图片且带方向时走「平移切换」（旧图滑出 + 新图滑入），其余场景保持淡入。
function showLbImage(dir) {
  const f = previewFiles[lbIndex];
  const isVid = isVideoExt(f.ext);
  const lbImg = $('lbImg');
  const lbVideo = $('lbVideo');
  const wasImgShown = lbImg.style.display !== 'none' && !!lbImg.getAttribute('src');
  lbImg.style.display = isVid ? 'none' : 'block';
  lbVideo.style.display = isVid ? 'block' : 'none';
  if (isVid) {
    lbVideo.src = f.previewUrl;
  } else {
    // 防御：上一次平移若被打断（如滑入中途关闭灯箱），内联样式可能残留
    //（内联优先级高于 .loaded，残留会让图偏移/透明）→ 先清掉再走各分支。
    lbImg.style.transition = '';
    lbImg.style.transform = '';
    lbImg.style.opacity = '';
    const newSrc = f.previewUrl;
    if (dir && wasImgShown && lbImg.getAttribute('src') !== newSrc) {
      slideToImage(dir, newSrc);                 // 平移切换：旧图滑出 + 新图滑入
      preloadNeighbors();
    } else if (lbImg.getAttribute('src') === newSrc) {
      lbImg.classList.add('loaded');             // 同一张（如重新打开），直接显示，避免卡在透明态
    } else {
      lbImg.classList.remove('loaded');         // 先淡出，加载完成再淡入（消除翻页闪动）
      lbImg.onload = () => { lbImg.classList.add('loaded'); };
      lbImg.src = newSrc;
      preloadNeighbors();                        // 预加载相邻图片，左右翻页秒出
    }
  }
  // 仅 1 个文件时隐藏左右箭头
  const showNav = previewFiles.length > 1;
  $('lbPrev').style.display = showNav ? 'flex' : 'none';
  $('lbNext').style.display = showNav ? 'flex' : 'none';
  // 重置顶部下载按钮为图标态（飞牛客户端里保持隐藏）
  const dlBtn = $('lbDownload');
  dlBtn.classList.remove('loading');
  dlBtn.disabled = false;
  dlBtn.style.display = hideDownload ? 'none' : '';
  dlBtn.innerHTML = SVG_DOWNLOAD;
  // 删除按钮：无管理权限（局域网设备直连）时隐藏（切换图片时同步显隐）
  $('lbDelete').style.display = canManage ? '' : 'none';
}

// ---- 灯箱「平移切换」 ----
// 时长；CSS 里 .lb-ghost 的过渡参数与此保持一致（LB_SLIDE_MS）。
const LB_SLIDE_MS = 260;
// 位移 = 图片宽度的 12%，clamp 在 48~120px：小图不过冲、大图也有明显的平移感。
const LB_SLIDE_RATIO = 0.12;
const LB_SLIDE_MIN = 48;
const LB_SLIDE_MAX = 120;

// 把当前图定格为幽灵层（.lb-ghost）向反方向滑出并淡出，新图从 dir 方向滑入
//（+1 = 新图从右侧进入，-1 = 从左侧）。灯箱大图居中且四周留黑，整屏滑动会有
// 空档感，小幅平移（按图宽比例）+ 渐隐更接近相册翻页的手感。
let lbSlideSeq = 0;                            // 平移切换代次：仅最新一次动画的收尾逻辑生效
function slideToImage(dir, newSrc) {
  const seq = ++lbSlideSeq;
  const lbImg = $('lbImg');
  const lb = $('lightbox');
  // 1) 定格当前图：按实际显示位置/尺寸复制一份（fixed 矩形），随后滑出
  const r = lbImg.getBoundingClientRect();
  const shift = Math.max(LB_SLIDE_MIN, Math.min(LB_SLIDE_MAX, Math.round(r.width * LB_SLIDE_RATIO)));
  const ghost = document.createElement('img');
  ghost.className = 'lb-ghost';
  ghost.src = lbImg.getAttribute('src');
  ghost.style.left = r.left + 'px';
  ghost.style.top = r.top + 'px';
  ghost.style.width = r.width + 'px';
  ghost.style.height = r.height + 'px';
  lb.appendChild(ghost);
  // 2) 主图瞬时置于滑入起点（不可见），下一帧起动画（双 rAF 保证起点已上屏）
  lbImg.onload = null;                           // 滑入期间不需要 onload 兜底（结束统一补 loaded）
  lbImg.classList.remove('loaded');
  lbImg.style.transition = 'none';
  lbImg.style.transform = 'translateX(' + (dir * shift) + 'px)';
  lbImg.style.opacity = '0';
  lbImg.src = newSrc;
  requestAnimationFrame(() => requestAnimationFrame(() => {
    ghost.style.transform = 'translateX(' + (-dir * shift) + 'px)';
    ghost.style.opacity = '0';
    lbImg.style.transition = 'transform ' + LB_SLIDE_MS + 'ms ease, opacity ' + LB_SLIDE_MS + 'ms ease';
    lbImg.style.transform = 'translateX(0)';
    lbImg.style.opacity = '1';
  }));
  // 3) 幽灵层移除（transitionend + 超时双保险）
  const dropGhost = () => ghost.remove();
  ghost.addEventListener('transitionend', dropGhost, { once: true });
  setTimeout(dropGhost, LB_SLIDE_MS + 120);
  // 4) 主图收尾：清内联、回 .loaded 淡入态。用代次（seq）保证只有最新一次动画的
  //    收尾生效；transitionend 在动画被打断（如滑入中途关闭灯箱）时不触发，
  //    故再加一道超时兜底，避免内联样式残留导致下次打开时图偏移/透明。
  const finishSlide = () => {
    lbImg.style.transition = '';
    lbImg.style.transform = '';
    lbImg.style.opacity = '';
    lbImg.classList.add('loaded');
  };
  lbImg.addEventListener('transitionend', function onSlideEnd(ev) {
    if (ev.target !== lbImg || ev.propertyName !== 'transform') return;
    lbImg.removeEventListener('transitionend', onSlideEnd);
    if (seq === lbSlideSeq) finishSlide();
  });
  setTimeout(() => { if (seq === lbSlideSeq) finishSlide(); }, LB_SLIDE_MS + 150);
}

// 灯箱内下载当前文件：与列表单文件下载同一套逻辑（HEAD 探活 + 直接导航），
// 下载中禁用按钮显示 loading。批量 ZIP 仍走 fetch+blob（POST 无法直接导航）。
async function lbDownload() {
  const btn = $('lbDownload');
  if (btn.classList.contains('loading')) return;
  const f = previewFiles[lbIndex];
  btn.classList.add('loading');
  btn.disabled = true;
  btn.innerHTML = '<span class="spinner"></span>';
  // 下载当前文件：优先用 downloadUrl（icns 等转换预览格式取原文件），
  // 其余格式从 previewUrl 去掉 inline 标记即为下载地址
  const dlUrl = f.downloadUrl || f.previewUrl.replace('&inline=1', '');
  try {
    if (await probeDownload(dlUrl)) downloadUrl(dlUrl);
  } finally {
    btn.classList.remove('loading');
    btn.disabled = false;
    btn.innerHTML = SVG_DOWNLOAD;
  }
}

// 灯箱内删除当前文件：确认后调 /api/delete；成功后重拉当前页——
// 列表自动补位（第 2 页首个滑入本页，与卡片删除同一机制）、分页器与计数刷新
//（本页删空会自动回退到最后一页）；再在新一轮预览列表里定位“原下一张”继续预览，
// 没有可预览项则关闭灯箱。
async function lbDelete() {
  if (lbIndex < 0 || lbIndex >= previewFiles.length) return;
  const f = previewFiles[lbIndex];
  if (!f) return;
  // 同卡片删除：弹窗展示名，请求用磁盘原名
  const ok = await confirmDialog({ title: '删除文件', desc: '确定删除「' + displayName(f.name) + '」？此操作不可恢复。', confirmText: '删除', cancelText: '取消', destructive: true });
  if (!ok) return;
  // 删除前记录继续预览的目标（优先下一张，其次上一张）：重拉后按名字定位
  const nextName = (previewFiles[lbIndex + 1] || previewFiles[lbIndex - 1] || {}).name || null;
  const okDel = await apiDelete(currentDir, f.name);
  if (!okDel) return;
  await openDir(currentDir, filesPage);
  loadDirs(false); // 刷新文件夹列表的文件计数
  // 在新预览列表里定位继续预览的图片；找不到（目录已空）则关闭灯箱
  const idx = nextName ? previewFiles.findIndex(p => p.name === nextName) : -1;
  if (idx < 0) { closeLightbox(); return; }
  lbIndex = idx;
  showLbImage();
}

function lbKeyHandler(e) {
  if (e.key === 'ArrowLeft') lbNav(-1);
  else if (e.key === 'ArrowRight') lbNav(1);
  else if (e.key === 'Escape') closeLightbox();
}

async function backToDirs() { sessionStorage.removeItem('aellus_currentDir'); await loadDirs(false); show('dirs'); }

// 文件数：超过 1 万则换算成 “X.X 万”
function formatCount(n) {
  if (n >= 10000) return (n/10000).toFixed(1) + '万';
  return String(n);
}
// 注：formatSize / formatDay / formatTime 已统一到 ui.js 共享层（全站同一实现）
// 属性值转义：直接复用 ui.js 的 escapeHtml（已转义 & < > " '）。
// 此前只转义 "，而服务端允许 & 出现在文件名里——含 & 的名字写进属性后，
// dataset.name 解出来的会是与磁盘不一致的值，删除 / 下载就会命中错误目标。
function escapeAttr(s) { return escapeHtml(s); }
// jsLit：生成可安全放进 HTML 内联事件处理器（onclick="..."）里的 JS 字符串字面量。
// 先用 JSON.stringify 做 JS 层转义（处理 ' " \ 及控制字符），再把 " 转成 &quot; 适配外层双引号属性。
// 仅用于 onclick="fn(${jsLit(x)})" 这类「把用户数据作为 JS 字符串参数」的场景；
// 普通属性值（如 data-name="..."）仍用 escapeAttr 即可。
function jsLit(s) {
  return JSON.stringify(String(s == null ? '' : s)).replace(/&/g, '&amp;').replace(/"/g, '&quot;');
}

// 下载类请求的统一前置：fetch → 非 ok / 网络异常时弹 toast 并返回 null，
// 正常返回 blob。调用方只需判空，错误提示与 try/catch 不再各处重复。
async function fetchBlob(url, init) {
  try {
    const res = await fetch(url, init);
    if (!res.ok) { toast('下载失败: ' + res.status, { variant: 'destructive' }); return null; }
    return await res.blob();
  } catch (e) {
    toast('下载失败: ' + e.message, { variant: 'destructive' });
    return null;
  }
}

// 触发「直接导航」下载：把浏览器 / 客户端自己的下载管理器拉起来。
// 单文件下载不用 fetch + blob，原因有三：
//  1. res.blob() 要把整个文件读进 JS 内存——几百 MB 的视频会把标签页直接撑爆；
//     而 /api/download 支持 Range，直连是流式的，内存恒定。
//  2. blob: URL 在部分 WebView（飞牛客户端、门户内嵌 iframe）里没有对应的下载
//     处理器，a.click() 后毫无反应——而服务端早已把 Content-Disposition 配好，
//     直接导航才是这些环境唯一走得通的路径，也正是「分享」二维码一直在用的方式。
//  3. 同源直连还能顺带做 HEAD 探活（见调用方），失败时给 toast，而不是把用户
//     导航到一个服务端错误页上去。
// 文件名由服务端的 Content-Disposition 决定（已去掉时间戳前缀，与页面展示一致），
// 故这里无需传文件名——a.download 对跨源 URL 会被浏览器忽略，传了也无效。
function downloadUrl(url) {
  const a = document.createElement('a');
  a.href = url;
  a.style.display = 'none';
  document.body.appendChild(a); // 必须先入文档：游离元素上的 click 在部分内核不触发下载
  a.click();
  // 立即移除不影响已发起的导航（同源 attachment 下载不会离开当前页）。
  setTimeout(() => a.remove(), 0);
}

// 公共：触发浏览器下载一个已在内存里的 blob。
// 仅批量打包 ZIP 用（POST /api/download-batch，无法直接导航，故必须走 blob）。
// 两处细节写错就会「点了没反应」，在部分 WebView 上尤其明显：
//  1. revokeObjectURL 不能紧跟 click() 同步执行——下载是异步开始的，同步撤销会让
//     浏览器在真正开始读取之前就失去数据源，下载被静默取消；
//  2. 元素要先挂进 document 再 click——游离元素上的 click 在部分内核不触发下载。
// 故：入文档 → click → 延迟释放（60s 足够任何下载管理器接手）。
function downloadBlob(blob, filename) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.style.display = 'none';
  document.body.appendChild(a);
  a.click();
  setTimeout(() => {
    a.remove();
    URL.revokeObjectURL(url);
  }, 60000);
}

// 公共：调用 /api/delete，成功返回 true，失败弹 toast 并返回 false
async function apiDelete(dir, name) {
  try {
    const res = await fetch('api/delete', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ dir: dir, file: name }),
    });
    if (!res.ok) {
      const d = await res.json().catch(() => ({}));
      toast('删除失败: ' + (d.error || res.status), { variant: 'destructive' });
      return false;
    }
    return true;
  } catch (e) {
    toast('删除失败: ' + e.message, { variant: 'destructive' });
    return false;
  }
}

// 启动：加载目录列表。原先此处会先「探测是否飞牛客户端」再决定是否隐藏下载入口，
// 该探测已随直连下载方案一并移除（hideDownload 恒为 false）。
(async function () {
  loadDirs();
})();

// 滚动吸顶毛玻璃：未滚动时 nav 与 batch-bar 分开、无背景；一旦下滑即整条满宽模糊
const topBars = document.querySelectorAll('.top-bar');
function syncTopBarBlur() {
  const scrolled = window.scrollY > 0;
  topBars.forEach(b => b.classList.toggle('scrolled', scrolled));
}
// 返回顶部：滚动超过一屏的少量距离后浮现（点击平滑回顶）
function scrollToTop() {
  window.scrollTo({ top: 0, behavior: 'smooth' });
}
function syncToTop() {
  $('toTop').classList.toggle('show', window.scrollY > 320);
}
window.addEventListener('scroll', function () { syncTopBarBlur(); syncToTop(); }, { passive: true });
syncTopBarBlur();
syncToTop();
