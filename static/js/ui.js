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
  window.ui = { toast, confirmDialog, escapeHtml, lockScroll, unlockScroll, getDeviceID, $, formatSize, formatDay, formatTime };
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
})();
