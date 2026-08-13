//go:build darwin && cgo

// macOS 菜单栏常驻：状态栏图标 + 「打开页面 / 退出」菜单 + 启动 toast 通知。
// 双击 .app 启动（非终端）时走此模式，HTTP 服务器在后台 goroutine 运行，
// 本函数阻塞在 NSApplication run loop，直到用户点击「退出」。
package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework UserNotifications
#import <Cocoa/Cocoa.h>
#import <UserNotifications/UserNotifications.h>

// 全局引用，防止状态栏与菜单 target 被提前释放
static NSStatusItem *g_statusItem = nil;
static NSObject     *g_target     = nil;

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
    completionHandler(UNNotificationPresentationOptionBanner);
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
*/
import "C"

import (
	"unsafe"
)

// menuBarEnabled 标记状态栏能力是否可用（cgo 编译时可用）。
func menuBarEnabled() bool { return true }

// runMenuBar 发启动通知并阻塞运行菜单栏常驻，直到用户点击「退出」。
func runMenuBar(saveDir, accessURL string) {
	cSaveDir := C.CString(saveDir)
	cURL := C.CString(accessURL)
	defer C.free(unsafe.Pointer(cSaveDir))
	defer C.free(unsafe.Pointer(cURL))
	C.aellusRunMenuBar(cSaveDir, cURL)
}
