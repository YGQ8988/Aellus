/*! 飞牛官方「授权目录」能力封装
 *
 * 背景：应用以独立包用户运行，要读写某个目录，必须由系统把该目录的 ACL 授予
 * 应用用户。手填路径即便拿到了也写不进去，所以必须走官方选择器
 * （JS SDK 的 pickSharedFile），由系统完成授权。
 *
 * 本模块只做两件事：
 *   1. 加载 @trimjs/web-app 并实例化 TrimApp
 *   2. 暴露 authorizeSaveDir()：弹官方选择器 -> 授权 -> 保存为 Aellus 保存目录
 *
 * 供首页设置面板的「＋ 添加授权目录」按钮调用（window.aellusAuthorizeSaveDir）。
 * 依赖：./vendor/trim-app.js（@trimjs/web-app 0.4.2，ESM，零外部依赖）
 * 前置：manifest 的 micro_app=true，且系统 >= 1.2.0401。
 */
import { TrimApp } from './vendor/trim-app.js';

var sdk = null;
function ensureSDK() {
  if (!sdk) { sdk = new TrimApp(); }
  return sdk;
}

// 判断是否为「用户取消」：宿主点了取消/关闭时，并不总是返回 undefined，
// 实测会返回失败码 + "Operation failed"（英文，与界面语言无关），也可能带
// cancel / dismiss / 取消 等字样。这些都不应弹红色错误，静默复原即可。
// 真实错误（例如非管理员的「仅管理员可进行此操作」）不含这些关键词，仍会正常提示。
function isCancelResult(res) {
  if (!res) { return true; }
  if (res.code === 0) { return false; }
  var m = String(res.msg || '').toLowerCase();
  if (!m) { return true; }                       // 失败码且无说明，多半是取消
  return m.indexOf('operation failed') >= 0
      || m.indexOf('cancel') >= 0
      || m.indexOf('dismiss') >= 0
      || m.indexOf('abort') >= 0
      || m.indexOf('取消') >= 0;
}

// 核心：弹出官方目录选择器 -> 系统授予 ACL -> 保存为 Aellus 保存目录。
// 返回结果对象：{ok, dir, display, msg, cancelled}；cancelled 表示用户主动取消。
async function authorizeSaveDir() {
  try {
    var res = await ensureSDK().pickSharedFile({
      title: '选择文件保存位置',
      okText: '授权并使用'
    });
    // 用户取消：静默复原（不弹红色错误），仅在控制台留痕便于排查
    if (isCancelResult(res)) {
      if (res && res.msg) { console.warn('[trim-auth] 授权已取消：', res.msg); }
      return { ok: false, cancelled: true, msg: '' };
    }
    // 真实失败：非管理员会返回「仅管理员可进行此操作」等，保留提示给用户
    if (res.code !== 0 || !res.data || !res.data.length) {
      return { ok: false, msg: res.msg || '未选择目录' };
    }
    var dir = res.data[0];
    var r = await fetch('api/set-savedir', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ dir: dir })
    });
    var d = await r.json().catch(function () { return {}; });
    if (d && d.ok) {
      return { ok: true, dir: d.saveDir, display: d.saveDirDisplay || d.saveDir };
    }
    return { ok: false, msg: (d && d.error) || '保存失败' };
  } catch (e) {
    return { ok: false, msg: '调用失败：' + ((e && e.message) || e) };
  }
}

// 暴露给首页内联脚本：设置面板的「＋ 添加授权目录」按钮
window.aellusAuthorizeSaveDir = authorizeSaveDir;
