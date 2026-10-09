/* ============================================================
   Aellus · 共享 UI 交互层（shadcn 风格）
   - toast()        Sonner 风格轻提示（替换原生 alert）
   - confirmDialog() shadcn Alert Dialog（替换原生 confirm，返回 Promise）
   ============================================================ */
(function () {
  'use strict';

  const SVG_INFO = '<svg class="toast-ico icon" viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>';
  const SVG_ERR  = '<svg class="toast-ico icon" viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>';
  const SVG_OK   = '<svg class="toast-ico icon" viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>';

  let viewport = null;
  let dialogRoot = null;

  // ---------------- 背景滚动锁 ----------------
  // html 因 `overflow-x: clip` 成为实际滚动容器（overflow-y 计算为 auto），
  // 只锁 body 无效，必须同时锁 html。用引用计数支持嵌套（灯箱内再开确认弹窗）。
  let scrollLockCount = 0;
  function lockScroll() {
    scrollLockCount++;
    document.documentElement.style.overflow = 'hidden';
    document.body.style.overflow = 'hidden';
  }
  function unlockScroll() {
    if (scrollLockCount > 0) scrollLockCount--;
    if (scrollLockCount === 0) {
      document.documentElement.style.overflow = '';
      document.body.style.overflow = '';
    }
  }

  function ensureViewport() {
    if (!viewport) {
      viewport = document.createElement('div');
      viewport.className = 'toast-viewport';
      viewport.setAttribute('aria-live', 'polite');
      document.body.appendChild(viewport);
    }
    return viewport;
  }
  function ensureDialogRoot() {
    if (!dialogRoot) {
      dialogRoot = document.createElement('div');
      dialogRoot.id = 'alert-dialog-root';
      document.body.appendChild(dialogRoot);
    }
    return dialogRoot;
  }

  // ---------------- Toast ----------------
  // toast(message, { variant:'default'|'destructive'|'success', title, duration })
  function toast(message, opts) {
    opts = opts || {};
    const variant = opts.variant || 'default';
    const duration = opts.duration != null ? opts.duration : 3200;
    const vp = ensureViewport();
    const el = document.createElement('div');
    el.className = 'toast' + (variant === 'destructive' ? ' destructive' : '');
    let ico = SVG_INFO;
    if (variant === 'destructive') ico = SVG_ERR;
    else if (variant === 'success') ico = SVG_OK;
    el.innerHTML =
      ico +
      '<div style="flex:1;min-width:0">' +
        (opts.title ? '<div class="toast-title">' + escapeHtml(opts.title) + '</div>' : '') +
        '<div class="toast-desc">' + escapeHtml(message) + '</div>' +
      '</div>' +
      '<button class="toast-close" aria-label="关闭">✕</button>';
    vp.appendChild(el);
    let timer = setTimeout(close, duration);
    function close() {
      clearTimeout(timer);
      el.classList.add('leaving');
      setTimeout(() => { if (el.parentNode) el.parentNode.removeChild(el); }, 180);
    }
    el.querySelector('.toast-close').addEventListener('click', close);
    return { close: close };
  }

  // ---------------- Alert Dialog ----------------
  // confirmDialog({ title, desc, confirmText, cancelText, destructive }) -> Promise<boolean>
  function confirmDialog(opts) {
    opts = opts || {};
    return new Promise(resolve => {
      const root = ensureDialogRoot();
      const overlay = document.createElement('div');
      overlay.className = 'alert-dialog-overlay modal-overlay';
      const destructive = !!opts.destructive;
      overlay.innerHTML =
        '<div class="alert-dialog modal-card" role="alertdialog" aria-modal="true">' +
          '<div class="alert-dialog-header">' +
            '<div class="alert-dialog-title">' + escapeHtml(opts.title || '确认操作') + '</div>' +
            (opts.desc ? '<div class="alert-dialog-desc">' + escapeHtml(opts.desc) + '</div>' : '') +
          '</div>' +
          '<div class="alert-dialog-footer">' +
            '<button class="btn btn-outline" data-act="cancel">' + escapeHtml(opts.cancelText || '取消') + '</button>' +
            '<button class="btn ' + (destructive ? 'btn-destructive' : 'btn-primary') + '" data-act="ok">' + escapeHtml(opts.confirmText || '确定') + '</button>' +
          '</div>' +
        '</div>';
      root.appendChild(overlay);
      // 弹窗打开期间锁定背景滚动（html/body 双锁，含 iOS 兼容）
      lockScroll();
      // 触发进入动画
      requestAnimationFrame(() => { overlay.dataset.open = 'true'; });

      function cleanup() {
        unlockScroll();
        overlay.dataset.open = 'false';
        setTimeout(() => { if (overlay.parentNode) overlay.parentNode.removeChild(overlay); }, 160);
        document.removeEventListener('keydown', onKey, true);
      }
      function done(val) { cleanup(); resolve(val); }
      function onKey(e) {
        if (e.key === 'Escape') { e.preventDefault(); e.stopPropagation(); done(false); }
        else if (e.key === 'Enter') { e.preventDefault(); e.stopPropagation(); done(true); }
      }
      overlay.addEventListener('click', e => {
        if (e.target === overlay) done(false);
        const act = e.target.getAttribute && e.target.getAttribute('data-act');
        if (act === 'cancel') done(false);
        else if (act === 'ok') done(true);
      });
      document.addEventListener('keydown', onKey, true);
    });
  }

  // ---------------- 共享工具（各页脚本复用，避免各自重复实现） ----------------
  const $ = id => document.getElementById(id);

  function escapeHtml(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  }

  // 文件大小：B / KB / MB / GB / TB（1024 进制）
  function formatSize(b) {
    if (b < 1024) return b + ' B';
    if (b < 1048576) return (b / 1024).toFixed(1) + ' KB';
    if (b < 1073741824) return (b / 1048576).toFixed(2) + ' MB';
    if (b < 1099511627776) return (b / 1073741824).toFixed(2) + ' GB';
    return (b / 1099511627776).toFixed(2) + ' TB';
  }
  // 仅日期：2026-08-16
  function formatDay(ts) {
    const d = new Date(ts * 1000);
    const p = n => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
  }
  // 日期 + 时分：2026-08-16 14:30
  function formatTime(ts) {
    const d = new Date(ts * 1000);
    const p = n => String(n).padStart(2, '0');
    return formatDay(ts) + ' ' + p(d.getHours()) + ':' + p(d.getMinutes());
  }

  // ---------------- 可预览格式清单（上传页 / 读取页共用的唯一真源） ----------------
  // 两页曾各自判一套：上传页看浏览器给的 MIME（image/* 、video/*），读取页看写死的
  // 扩展名清单，两边不一致 → 同一格式在上传页能出预览、读取页却只显示扩展名占位。
  // 现在两侧都从这里取，加格式只改这一处。
  //
  // 两条例外，各自单独走一条路径：
  //   - icns：容器格式，浏览器渲染不出来。上传页本地解容器取内嵌图像，
  //     读取页走服务端 /api/thumb 转 PNG。不属于「<img> 直接能渲染」的范畴，不入表。
  //   - svg：服务端对可携带脚本的类型一律不内联（防存储型 XSS，见 Go 端
  //     fileout.go），读取页因此取回字节后自行以 image/svg+xml 建 blob 再交给 <img>
  //     —— 与上传页「本地 blob + <img>」是同一条安全路径：SVG 经 <img> 渲染时
  //     浏览器走「安全静态模式」，不执行脚本、不加载外部引用。
  const PREVIEW_IMG_EXTS = ['png', 'jpg', 'jpeg', 'jfif', 'gif', 'webp', 'bmp', 'ico', 'heic', 'heif', 'avif', 'tif', 'tiff', 'svg'];
  // 视频清单与后端 isInlineSafe 的视频部分对齐（mp4/webm/mov/avi/mkv/m4v）：
  // 读取页用 <video src=原始下载链接>，后端不放行的话只会拿到附件流。
  const PREVIEW_VID_EXTS = ['mp4', 'm4v', 'mov', 'webm', 'avi', 'mkv'];

  // 扩展名 -> MIME。用途有两个：
  //   1) 部分系统/浏览器给不出 File.type（如 Windows 上的 .heic、某些客户端里的 .mkv），
  //      这时 MIME 为空，建出来的 blob / data URL 没有类型，浏览器不会渲染；
  //   2) 读取页取 svg 时服务端返回的是 application/octet-stream，必须显式给出类型。
  const EXT_MIME = {
    png: 'image/png', jpg: 'image/jpeg', jpeg: 'image/jpeg', jfif: 'image/jpeg',
    gif: 'image/gif', webp: 'image/webp', bmp: 'image/bmp', ico: 'image/x-icon',
    heic: 'image/heic', heif: 'image/heif', avif: 'image/avif',
    tif: 'image/tiff', tiff: 'image/tiff', svg: 'image/svg+xml',
    mp4: 'video/mp4', m4v: 'video/x-m4v', mov: 'video/quicktime',
    webm: 'video/webm', avi: 'video/x-msvideo', mkv: 'video/x-matroska'
  };

  // 取扩展名（小写，不含点）。".bashrc" 这类隐藏文件视为无扩展名（返回 ''）。
  function extOf(name) {
    const s = String(name == null ? '' : name);
    const i = s.lastIndexOf('.');
    return i > 0 ? s.slice(i + 1).toLowerCase() : '';
  }

  // 是否按「图片」预览：优先信浏览器给的 MIME，给不出时按扩展名兜底。
  function isPreviewImage(f) {
    if (!f) return false;
    if (String(f.type || '').indexOf('image/') === 0) return true;
    return PREVIEW_IMG_EXTS.indexOf(extOf(f.name)) >= 0;
  }
  // 是否按「视频」预览，判定同上。
  function isPreviewVideo(f) {
    if (!f) return false;
    if (String(f.type || '').indexOf('video/') === 0) return true;
    return PREVIEW_VID_EXTS.indexOf(extOf(f.name)) >= 0;
  }

  // ---------------- 展示名：去掉上传时拼接的时间戳前缀 ----------------
  // 本工具「普通文件」上传时会给磁盘名加时间戳前缀（20060102_150405.000000_，
  // 见 Go 端 resolveUploadTarget）避免重名覆盖。三个页面（上传页结果卡、读取页卡片、
  // 删除确认弹窗）统一在这里去掉该前缀，只显示实际上传时的文件名；非本工具上传
  //（无此前缀）的名字原样返回。磁盘真实名不变——下载 / 删除 / 缩略图仍用原名，
  // 只有「给人看」的地方走这个函数。
  //
  // 前缀只可能出现在名字开头，故正则锚定 ^；文件夹上传的 displayName 带目录层级
  //（"dir/sub/file.jpg"），开头是目录名，不会被误伤。
  const TS_PREFIX_RE = /^\d{8}_\d{6}\.\d{6}_/;
  function displayName(name) {
    const s = String(name == null ? '' : name);
    return TS_PREFIX_RE.test(s) ? s.replace(TS_PREFIX_RE, '') : s;
  }

  // ---------------- 设备 ID（仅用于设备名回填 / 访问日志） ----------------
  // 首次访问生成 UUID 存 localStorage，之后所有接口请求头自动携带 Deviceid。
  // 它只是功能标识，不参与任何权限判定：删除 / 改保存目录由服务端依据
  // 「网关注入的身份头」或「TCP 源地址是否本机」判定（见 Go 端 canManage），
  // 前端伪造该值不会带来任何权限。
  function uuidv4() {
    if (window.crypto && typeof crypto.randomUUID === 'function') {
      return crypto.randomUUID();
    }
    return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function (c) {
      var r = (Math.random() * 16) | 0;
      var v = c === 'x' ? r : (r & 0x3) | 0x8;
      return v.toString(16);
    });
  }

  function getDeviceID() {
    try {
      var id = localStorage.getItem('aellus_device_id');
      if (id) return id;
      id = uuidv4();
      localStorage.setItem('aellus_device_id', id);
      return id;
    } catch (e) {
      return '';
    }
  }

  // 注意：本文件只负责 UI 交互与请求头注入，不做任何权限判定。
  // 权限（删除 / 改保存目录）完全由服务端决定（见 Go 端 canManage：只认飞牛统一网关
  // 注入的身份头，或来自本机的请求）；前端判定可被伪造，不得采信。
  // 同样不要在这里加载第三方脚本——同源执行的每一段外部代码都是潜在风险。

  // 全局 fetch 拦截：自动给所有请求加 Deviceid 头（设备名映射 / 访问日志用）与
  // X-Aellus-Client 头（服务端据此拒绝跨站请求，见 Go 端 requireTrustedClient）。
  var origFetch = window.fetch;
  window.fetch = function (url, options) {
    options = options || {};
    var id = getDeviceID();
    var headers = options.headers;
    if (headers instanceof Headers) {
      if (id && !headers.has('Deviceid')) headers.set('Deviceid', id);
      if (!headers.has('X-Aellus-Client')) headers.set('X-Aellus-Client', '1');
    } else {
      var h = {};
      if (headers && typeof headers === 'object') {
        for (var k in headers) {
          if (Object.prototype.hasOwnProperty.call(headers, k)) h[k] = headers[k];
        }
      }
      if (id && !('Deviceid' in h)) h['Deviceid'] = id;
      if (!('X-Aellus-Client' in h)) h['X-Aellus-Client'] = '1';
      options.headers = h;
    }
    return origFetch.call(window, url, options);
  };

  // 诊断日志（默认关闭）：输出本机 IP 与服务端 IP / 运行环境，便于排查删除权限等
  // 「本机 / 飞牛环境」判定问题。默认不打印——控制台里的内网地址会在共享屏幕、录屏
  // 或用户贴控制台日志时泄露内网拓扑。需要排查时在控制台执行
  // localStorage.setItem('aellus_debug','1') 再刷新页面即可开启。
  try {
    if (localStorage.getItem('aellus_debug')) {
      fetch('api/addr').then(function(r){ return r.json(); }).then(function(d){
        console.log('[Aellus] 本机 IP（当前访问设备）: ' + (d.clientIP || '未知'));
        console.log('[Aellus] 服务端 IP（运行 Aellus 的设备）: ' + (d.ip || '未知'));
        console.log('[Aellus] 服务端运行环境: ' + (d.platform || '未知'));
      }).catch(function(){});
    }
  } catch (e) {}

  // 暴露到全局：同时挂到 window.ui 命名空间与顶层全局，
  // 兼容以裸名（toast() / confirmDialog()）调用的业务代码。
  window.ui = { toast, confirmDialog, escapeHtml, lockScroll, unlockScroll, getDeviceID, $, formatSize, formatDay, formatTime,
                PREVIEW_IMG_EXTS, PREVIEW_VID_EXTS, EXT_MIME, extOf, isPreviewImage, isPreviewVideo,
                TS_PREFIX_RE, displayName };
  window.toast = toast;
  window.confirmDialog = confirmDialog;
  window.escapeHtml = escapeHtml;
  window.lockScroll = lockScroll;
  window.unlockScroll = unlockScroll;
  window.getDeviceID = getDeviceID;
  window.$ = $;
  window.formatSize = formatSize;
  window.formatDay = formatDay;
  window.formatTime = formatTime;
  window.PREVIEW_IMG_EXTS = PREVIEW_IMG_EXTS;
  window.PREVIEW_VID_EXTS = PREVIEW_VID_EXTS;
  window.EXT_MIME = EXT_MIME;
  window.extOf = extOf;
  window.isPreviewImage = isPreviewImage;
  window.isPreviewVideo = isPreviewVideo;
  window.TS_PREFIX_RE = TS_PREFIX_RE;
  window.displayName = displayName;
})();
