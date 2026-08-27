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

  // 暴露到全局：同时挂到 window.ui 命名空间与顶层全局，
  // 兼容以裸名（toast() / confirmDialog()）调用的业务代码。
  window.ui = { toast: toast, confirmDialog: confirmDialog, escapeHtml: escapeHtml, lockScroll: lockScroll, unlockScroll: unlockScroll };
  window.toast = toast;
  window.confirmDialog = confirmDialog;
  window.escapeHtml = escapeHtml;
  window.lockScroll = lockScroll;
  window.unlockScroll = unlockScroll;
})();
