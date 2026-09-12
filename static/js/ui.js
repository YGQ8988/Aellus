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
      overlay.className = 'alert-dialog-overlay';
      const destructive = !!opts.destructive;
      overlay.innerHTML =
        '<div class="alert-dialog" role="alertdialog" aria-modal="true">' +
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

  // 暴露到全局，供业务脚本（如 browse.js）复用，避免各自重复实现
  function escapeHtml(s) {
    return String(s == null ? '' : s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  }

  // ---------------- 设备 ID（设备名映射 / 访问日志用） ----------------
  // 首次访问生成 UUID 存 localStorage，之后所有接口请求头自动携带 Deviceid，
  // 服务端据此（连同 IP）判定文件是否可删，替代原 UA 设备签名。
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

  // 说明：这里原本会预加载飞牛官方 SDK（static/js/trim-web-app.js）以读取其
  // isStandaloneWeb 值，用于前端判断「当前是否为独立网页」。权限判定早已全部移到服务端
  // （只认飞牛统一网关注入的身份头，或来自本机的请求，见 Go 端 canManage），该值现已
  // 没有任何调用方，故整段删除：少加载一个第三方脚本，就少一份在应用同源里执行的外部代码。
  // 请勿在此恢复「前端判定权限」——它可被伪造，服务端也不会采信。

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

  // 诊断日志：控制台输出本机 IP（当前访问设备）与服务端 IP（运行 Aellus 的设备），
  // 便于排查删除权限等「本机 / 飞牛环境」判定问题（打开浏览器开发者工具控制台可见）。
  try {
    fetch('api/addr').then(function(r){ return r.json(); }).then(function(d){
      console.log('[Aellus] 本机 IP（当前访问设备）: ' + (d.clientIP || '未知'));
      console.log('[Aellus] 服务端 IP（运行 Aellus 的设备）: ' + (d.ip || '未知'));
      console.log('[Aellus] 服务端运行环境: ' + (d.platform || '未知'));
    }).catch(function(){});
  } catch (e) {}

  // 暴露到全局：同时挂到 window.ui 命名空间与顶层全局，
  // 兼容以裸名（toast() / confirmDialog()）调用的业务代码。
  window.ui = { toast: toast, confirmDialog: confirmDialog, escapeHtml: escapeHtml, lockScroll: lockScroll, unlockScroll: unlockScroll, getDeviceID: getDeviceID };
  window.toast = toast;
  window.confirmDialog = confirmDialog;
  window.escapeHtml = escapeHtml;
  window.lockScroll = lockScroll;
  window.unlockScroll = unlockScroll;
  window.getDeviceID = getDeviceID;
})();
