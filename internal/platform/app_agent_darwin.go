//go:build darwin && !fpk

package platform

/*
#cgo LDFLAGS: -framework AppKit
#include <objc/objc.h>
#include <objc/message.h>
#include <objc/runtime.h>

// 仅声明 AppKit 的 C 函数原型的「签名」，不 include AppKit.h，
// 避免 cgo 的 C 预编译块（.c）解析不了 Objective-C 头（@class/NSString 等会报语法错）。
// 链接阶段由 -framework AppKit 从系统框架解析符号，运行时 dyld 从共享缓存加载。
extern int NSApplicationLoad(void);

static void setAgentActivationPolicy(void) {
    // 本程序经 .app 的 universal 二进制直接启动（build-mac.sh 用 lipo 合并，
    // 不再需要 aellus-launcher 这类按架构选二进制的 shell 启动器），不属于「标准 GUI 启动」，
    // AppKit 不会像普通 GUI 应用那样自动完成加载。若不先加载，
    // objc_getClass("NSApplication") 会返回 NULL，导致 setActivationPolicy 整段静默失效、
    // 应用保持默认的 Regular(前台)策略，Dock 就会弹跳一次。
    // NSApplicationLoad() 是 Apple 官方给「非标准启动进程使用 NSApplication」的加载方式。
    NSApplicationLoad();

    Class appClass = objc_getClass("NSApplication");
    if (appClass == 0) return;
    SEL shared = sel_registerName("sharedApplication");
    id app = ((id (*)(Class, SEL))objc_msgSend)(appClass, shared);
    if (app == 0) return;
    // 再取一次，确保 sharedApplication 已实例化（触发 AppKit 完整初始化）
    app = ((id (*)(Class, SEL))objc_msgSend)(appClass, shared);
    if (app == 0) return;

    SEL setPolicy = sel_registerName("setActivationPolicy:");
    // NSApplicationActivationPolicyAccessory = 1
    //   (0=Regular 前台带 Dock 图标, 1=Accessory 菜单栏常驻无 Dock, 2=Prohibited 完全后台)
    ((void (*)(id, SEL, long))objc_msgSend)(app, setPolicy, 1);
}
*/
import "C"

// setAgentMode 将应用激活策略设为 Accessory（菜单栏常驻型），
// 避免作为前台应用被激活时 Dock 弹跳一次（"抖一下"）。
// 与 Info.plist 的 LSUIElement=true 配合，确保只在顶栏显示、Dock 不出现。
func setAgentMode() {
	C.setAgentActivationPolicy()
}
