//go:build darwin && cgo

// macOS 菜单栏常驻：状态栏图标 + 「打开页面 / 退出」菜单 + 启动 toast 通知。
// 双击 .app 启动（非终端）时走此模式，HTTP 服务器在后台 goroutine 运行，
// 本函数阻塞在 NSApplication run loop，直到用户点击「退出」。
package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework UserNotifications
#import <Cocoa/Cocoa.h>
#import <UserNotifications/UserNotifications.h>

// 全局引用，防止状态栏与菜单 target 被提前释放
static NSStatusItem *g_statusItem = nil;
static NSObject     *g_target     = nil;
static NSMenuItem   *g_openItem   = nil; // 「打开页面」项，后台刷新 IP 时更新其 URL

@interface AellusMenuTarget : NSObject <UNUserNotificationCenterDelegate>
- (void)openURL:(id)sender;
- (void)quit:(id)sender;
@end

@implementation AellusMenuTarget
- (void)openURL:(id)sender {
    NSString *url = [(NSMenuItem *)sender representedObject];
    if (url.length) {
        [[NSWorkspace sharedWorkspace] openURL:[NSURL URLWithString:url]];
    }
}
- (void)quit:(id)sender {
    [NSApp terminate:nil];
}
// 应用运行时也让通知以横幅形式弹出（而非仅进通知中心）
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
       willPresentNotification:(UNNotification *)notification
         withCompletionHandler:(void (^)(UNNotificationPresentationOptions))completionHandler {
    // Banner 常量需 macOS 11.0+，改用 Alert（10.14+，数值与 Banner 相同）以兼容 macOS 10.15
    completionHandler(UNNotificationPresentationOptionAlert);
}
@end

static void aellusRunMenuBar(const char *saveDirCStr, const char *urlCStr) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        // 作为 accessory 应用：不占 Dock，仅出现在菜单栏
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];

        AellusMenuTarget *target = [[AellusMenuTarget alloc] init];
        g_target = target;

        // 1. 启动 toast 通知（图标自动使用应用图标，与状态栏/App 图标统一）
        UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
        center.delegate = target;
        [center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert)
                             completionHandler:^(BOOL granted, NSError *error) {
            if (!granted) return;
            UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
            content.title = @"Aellus 已启动";
            content.body = [NSString stringWithFormat:
                @"文件存储目录: %@\n访问地址: %@\n后续也可在状态栏上操作打开页面和退出程序",
                [NSString stringWithUTF8String:saveDirCStr],
                [NSString stringWithUTF8String:urlCStr]];
            UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:@"aellus-start"
                                                                                 content:content
                                                                                 trigger:nil];
            [center addNotificationRequest:request withCompletionHandler:nil];
        }];

        // 2. 状态栏图标：使用应用图标（Aellus.icns），缩放到菜单栏尺寸
        g_statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
        NSImage *appIcon = [NSRunningApplication currentApplication].icon;
        if (!appIcon) {
            appIcon = [NSImage imageNamed:NSImageNameActionTemplate];
        }
        NSSize iconSize = NSMakeSize(18, 18);
        NSImage *scaledIcon = [[NSImage alloc] initWithSize:iconSize];
        [scaledIcon lockFocus];
        [appIcon drawInRect:NSMakeRect(0, 0, iconSize.width, iconSize.height)
                   fromRect:NSZeroRect
                  operation:NSCompositingOperationCopy
                   fraction:1.0];
        [scaledIcon unlockFocus];
        g_statusItem.button.image = scaledIcon;

        // 3. 菜单：「打开页面 / 退出」
        NSMenu *menu = [[NSMenu alloc] init];

        NSMenuItem *openItem = [[NSMenuItem alloc] initWithTitle:@"打开页面"
                                                          action:@selector(openURL:)
                                                   keyEquivalent:@""];
        openItem.target = target;
        openItem.representedObject = [NSString stringWithUTF8String:urlCStr];
        [menu addItem:openItem];
        g_openItem = openItem;

        [menu addItem:[NSMenuItem separatorItem]];

        NSMenuItem *quitItem = [[NSMenuItem alloc] initWithTitle:@"退出"
                                                          action:@selector(quit:)
                                                   keyEquivalent:@""];
        quitItem.target = target;
        [menu addItem:quitItem];

        g_statusItem.menu = menu;

        [NSApp run];
    }
}

// 更新菜单「打开页面」项的 URL（dispatch 到主线程：NSMenuItem 非线程安全）。
// 入参 urlCStr 由 Go 侧 C.CString 分配；本函数内立即 copy 成 NSString，故 Go 侧可安全 free。
static void aellusUpdateMenuURL(const char *urlCStr) {
    NSString *url = [NSString stringWithUTF8String:urlCStr];
    dispatch_async(dispatch_get_main_queue(), ^{
        if (g_openItem) {
            g_openItem.representedObject = url;
        }
    });
}

// IP 变化时发通知告知新地址（dispatch 到主线程：UNUserNotificationCenter 非线程安全）。
static void aellusNotifyURLChanged(const char *urlCStr) {
    NSString *url = [NSString stringWithUTF8String:urlCStr];
    dispatch_async(dispatch_get_main_queue(), ^{
        UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
        UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
        content.title = @"Aellus 访问地址已更新";
        content.body = [NSString stringWithFormat:@"检测到网络变化，新地址: %@", url];
        UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:@"aellus-url-change"
                                                                             content:content
                                                                             trigger:nil];
        [center addNotificationRequest:request withCompletionHandler:nil];
    });
}
*/
import "C"

import (
	"os"
	"strings"
	"time"
	"unsafe"
)

// MenuBarEnabled 判断是否可启用菜单栏常驻模式。
// macOS 菜单栏依赖 UNUserNotificationCenter，要求进程在合法 .app bundle 内运行；
// 裸二进制（如 /tmp/aellus、./aellus）无 bundle，强行走菜单栏会触发
// NSInternalInconsistencyException 崩溃。故用可执行路径是否位于
// .app/Contents/MacOS/ 下作为 bundle 合法性判据。
func MenuBarEnabled() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.Contains(exe, ".app/Contents/MacOS/")
}

// RunMenuBar 发启动通知并阻塞运行菜单栏常驻，直到用户点击「退出」。
//
// getURL 用于后台自动探测：每 60s 调用一次重新获取当前访问 URL，与上次不同则
// 更新菜单「打开页面」项的地址并发通知。HTTP 服务监听 0.0.0.0 不依赖具体 IP，
// 无需重启；getURL 返回空串表示当前离线/探测失败，跳过更新。
func RunMenuBar(saveDir, accessURL string, getURL func() string) {
	// 后台探测须先于阻塞的 NSApp run loop 启动，否则 goroutine 永不执行
	go func(current string) {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			newURL := getURL()
			if newURL == "" || newURL == current {
				continue
			}
			current = newURL
			c := C.CString(newURL)
			C.aellusUpdateMenuURL(c)
			C.aellusNotifyURLChanged(c)
			C.free(unsafe.Pointer(c))
		}
	}(accessURL)

	cSaveDir := C.CString(saveDir)
	cURL := C.CString(accessURL)
	defer C.free(unsafe.Pointer(cSaveDir))
	defer C.free(unsafe.Pointer(cURL))
	C.aellusRunMenuBar(cSaveDir, cURL)
}
