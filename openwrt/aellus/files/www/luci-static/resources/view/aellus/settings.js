'use strict';
'require view';
'require form';
'require uci';
'require rpc';

// Aellus —— LuCI 配置页面（菜单：服务 → Aellus局域网传输）
//
// 按 OpenWrt 24.10 官方的 JS view 规范编写：
//   视图   /www/luci-static/resources/view/aellus/settings.js（本文件）
//   菜单   /usr/share/luci/menu.d/luci-app-aellus.json
//   权限   /usr/share/rpcd/acl.d/luci-app-aellus.json
//
// 保存并应用后如何生效：
//   LuCI 应用 → ubus uci apply → /sbin/reload_config 发出 config.change 事件 →
//   procd 按 init 脚本里声明的 procd_add_reload_trigger aellus 执行
//   /etc/init.d/aellus reload → restart：enabled=1 则按新配置启动，enabled=0 则停止。

// —— 状态徽标 / 链接的 DOM 构造 ——
//
// 这里用原生 DOM API 造节点，而不是拼 HTML 字符串，原因有两个：
//   1) LuCI 的 E() 是内部工具函数，luci.js 只把 XHR 和 LuCI 挂到 window，
//      E / dom 都没导出到全局，第三方页面不该依赖它们；
//   2) dom.append() 遇到 Node 走 appendChild（luci.js 第 1327 行）、遇到字符串才走
//      innerHTML（第 1331 行）。所以把节点塞进 DummyValue 的 default，既能渲染出
//      带 target="_blank" 的 <a>，又天然免疫 XSS —— 主机名和端口永远只被当作文本。
//
// 不用 DummyValue 自带的 href 属性：官方实现只能产出不带 target 的 <a>
// （form.js 第 5456 行 outputEl.appendChild(E('a', { href: this.href }))），
// 两个机制一起用还会造成 <a> 嵌套。所以整枚链接自己构造。
//
// 副作用说明：DummyValue 会顺带渲染一个隐藏 input 存放 value（官方行为），
// 节点被塞进去后会显示为 "[object HTMLAnchorElement]"，但该 input 不可见，
// 且 DummyValue.write() 是空实现、不会把值写回 UCI，因此没有实际影响。

// 颜色直接写死而不用主题的 CSS 变量：openwrt-2020 主题是 --success-color /
// --danger-color，bootstrap 主题又是另一套名字，第三方主题各不一样，
// 写成 inline style 才能在任意主题下都稳定显示。
const STATUS_COLOR = {
	running: '#2f9e44',   // 运行中：绿
	stopped: '#e03131'    // 未运行：红
};

function makeStatusNode(running) {
	const el = document.createElement('span');
	el.setAttribute('style', 'color:' + (running ? STATUS_COLOR.running : STATUS_COLOR.stopped) +
		';font-weight:bold');
	el.appendChild(document.createTextNode(running ? _('运行中') : _('未运行')));
	return el;
}

function makeLinkNode(url) {
	const el = document.createElement('a');
	el.setAttribute('href', url);
	el.setAttribute('target', '_blank');
	// rel 必须带上：target="_blank" 打开的页面能通过 window.opener 反向操作本页，
	// 而本页正好是管理后台。noreferrer 顺带不再泄漏地址栏里的主机名。
	el.setAttribute('rel', 'noreferrer noopener');
	el.appendChild(document.createTextNode(url));
	return el;
}

// 服务运行状态：经 rpcd 调 ubus 的 service list 查询
// （对应 ACL 里的 read.ubus.service = ["list"]）
const callServiceList = rpc.declare({
	object: 'service',
	method: 'list',
	params: [ 'name', 'verbose' ]
});

// serviceRunning 判断 procd 里是否已有在运行的实例。
// 查不到（服务从未启动 / rpcd 未授权）时按「未运行」处理，不影响配置功能。
function serviceRunning(data) {
	try {
		let inst = data && data.aellus && data.aellus.instances;
		for (let k in (inst || {}))
			if (inst[k] && inst[k].running)
				return true;
	} catch (e) {
		/* 忽略：状态只是辅助展示 */
	}
	return false;
}

return view.extend({
	load: function () {
		return Promise.all([
			uci.load('aellus'),
			callServiceList('aellus', true).catch(function () { return null; })
		]);
	},

	render: function (data) {
		let m, s, o;
		let running = serviceRunning(data[1]);
		let port = uci.get('aellus', 'main', 'port') || '5115';
		// 访问地址：用当前 LuCI 页面所在的主机名（即路由器 LAN 地址）拼端口
		let url = 'http://' + window.location.hostname + ':' + port;

		m = new form.Map('aellus', _('Aellus局域网传输'),
			_('局域网文件互传：手机 / 电脑在同一局域网内用浏览器访问路由器端口，即可上传、浏览、下载文件。' +
			  '修改后点击「保存并应用」，服务会按新配置自动启动或停止。'));

		s = m.section(form.NamedSection, 'main', 'aellus', _('运行状态'));

		o = s.option(form.DummyValue, '_aellus_status', _('服务状态'));
		o.default = makeStatusNode(running);
		o.rmempty = false;
		o.description = running
			? _('服务已在后台运行，可直接使用下面的访问地址。')
			: _('服务未运行：勾选下方「启用服务」并「保存并应用」即可启动。');

		o = s.option(form.DummyValue, '_aellus_url', _('访问地址'));
		o.default = makeLinkNode(url);
		o.rmempty = false;
		o.description = _('点击会在新标签页打开；改动端口号后需「保存并应用」，链接才会跟着更新。');

		s = m.section(form.NamedSection, 'main', 'aellus', _('服务设置'));

		o = s.option(form.Flag, 'enabled', _('启用服务'),
			_('勾选并「保存并应用」后启动服务；取消勾选则停止服务。'));
		o.default = '1';
		o.rmempty = false;

		o = s.option(form.Value, 'port', _('服务端口号'),
			_('HTTP 监听端口（1-65535）。修改后需「保存并应用」才会生效。'));
		o.datatype = 'port';
		o.default = '5115';
		o.rmempty = false;

		o = s.option(form.Value, 'save_dir', _('文件储存目录'),
			_('上传文件的保存目录，建议指向外挂硬盘或 U 盘（如 /mnt/sda1/aellus）。' +
			  '留空则自动探测 /mnt、/media 下的可写挂载点。请勿指向 /root、/overlay 等闪存路径，' +
			  '路由器闪存容量小且写入寿命有限。'));
		o.placeholder = '/mnt/sda1/aellus';
		o.rmempty = true;

		s = m.section(form.NamedSection, 'main', 'aellus', _('高级'));

		o = s.option(form.ListValue, 'lang', _('控制台输出语言'),
			_('日志与控制台输出的语言；路由器终端通常缺中文字形，默认英文。'));
		o.value('en', _('English'));
		o.value('zh', _('中文'));
		o.default = 'en';
		o.rmempty = false;

		o = s.option(form.Value, 'gomemlimit', _('内存上限'),
			_('Go 运行时内存上限，如 96MiB / 64MiB。内存较小的机型（128MB）可下调。'));
		o.default = '96MiB';
		o.rmempty = true;

		return m.render();
	}
});
