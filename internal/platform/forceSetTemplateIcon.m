// forceSetTemplateIcon.m — compiled by cgo as Objective-C via #cgo directive
// 1) Bypasses systray's internal setIcon path to reliably render menu-bar icon on macOS 11-26.
// 2) Hooks an NSMenuDelegate onto systray's menu so hovering the "访问二维码" item
//    (after the menu is opened by clicking the tray icon) shows the QR popover —
//    matching LanCoClip's HoveredCh behaviour, since getlantern/systray has no per-item hover.
#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

// goQRHover 由 tray_darwin.go 通过 cgo //export 提供：菜单项被高亮（1）/取消高亮（0）时
// 由 Go 侧转调 qrpop.Show/Hide 来弹/收二维码浮窗。
void goQRHover(int hovered);

// QRMenuHoverDelegate：挂在 systray 的 NSMenu 上，监听菜单项高亮变化。
@interface QRMenuHoverDelegate : NSObject <NSMenuDelegate>
@end
@implementation QRMenuHoverDelegate
- (void)menu:(NSMenu *)menu willHighlightItem:(NSMenuItem *)item {
    // hover 到「访问二维码」项 -> 弹二维码；移到别处或高亮清空 -> 收起。
    if (item != nil && [[item title] isEqualToString:@"访问二维码"]) {
        goQRHover(1);
    } else {
        goQRHover(0);
    }
}
- (void)menuWillClose:(NSMenu *)menu {
    // 菜单收起时收起二维码浮窗（悬浮二维码是 hover 专属，点击走页面 ?qr=1）。
    goQRHover(0);
}
@end
static QRMenuHoverDelegate *gQRMenuDelegate = nil;

// setupQRMenuHover 给 systray 创建的 NSMenu 挂上 hover delegate。
// 必须在菜单对象已存在之后调用（systray 在 applicationDidFinishLaunching 里建好 menu，
// 早于 onReady），且按标题动态匹配「访问二维码」项，与菜单项何时被加入无关。
void setupQRMenuHover(void) {
    @autoreleasepool {
        id delegate = [[NSApplication sharedApplication] delegate];
        if (!delegate) return;
        NSStatusItem *statusItem = [delegate valueForKey:@"statusItem"];
        if (!statusItem) return;
        NSMenu *menu = statusItem.menu;
        if (!menu) return;
        if (!gQRMenuDelegate) {
            gQRMenuDelegate = [[QRMenuHoverDelegate alloc] init];
        }
        menu.delegate = gQRMenuDelegate;
    }
}

void forceSetTemplateIcon(const char *bytes, int length) {
    @autoreleasepool {
        // 1) Create NSImage from raw PNG bytes
        NSData *data = [NSData dataWithBytesNoCopy:(void *)bytes length:length freeWhenDone:NO];
        NSImage *image = [[NSImage alloc] initWithData:data];
        if (!image) return;

        // 2) Set 22pt logical size + template=YES
        [image setSize:NSMakeSize(22, 22)];
        [image setTemplate:YES];

        // 3) Get systray AppDelegate -> statusItem via KVC (private ivar)
        id delegate = [[NSApplication sharedApplication] delegate];
        if (!delegate) return;
        NSStatusItem *statusItem = [delegate valueForKey:@"statusItem"];
        if (!statusItem) return;

        // 4) Set button image directly
        [statusItem.button setImage:image];
        [statusItem.button setImagePosition:NSImageOnly]; // icon only, no text
    }
}
