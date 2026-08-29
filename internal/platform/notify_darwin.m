// notify_darwin.m — compiled by cgo as Objective-C via #cgo directive.
// Posts a macOS system notification on app launch; clicking it opens the browser.
// 若系统通知不可用（未授权 / 代理类 app 弹不出授权框），静默跳过——
// 菜单栏图标已是主要交互入口，不用模态弹窗打断用户。
//
// 重要：
// 1) 所有涉及 AppKit/Cocoa UI 的调用（runModal / requestAuthorization /
//    addNotificationRequest）都必须在【主线程】执行，否则会闪退。
//    UNUserNotificationCenter 的 completion handler 跑在后台线程，因此内部一律
//    用 onMain() 切回主线程再碰 UI。
// 2) title/body/url 来自 Go 侧 C.CString，Go 会在本函数返回后立即 free。
//    必须在这里【立刻】拷贝成 NSString（自带内存），后续所有异步回调都用 NSString，
//    否则会发生 use-after-free（表现为：首次偶发、二次必崩 / 点击打不开浏览器）。
#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>
#import <AppKit/AppKit.h>
#import <dispatch/dispatch.h>

// Provided by Go's //export aellusOpenBrowser (in notify_darwin.go)
extern void aellusOpenBrowser(const char* url);

@interface AellusNotifyDelegate : NSObject <UNUserNotificationCenterDelegate>
@end

@implementation AellusNotifyDelegate

// 用户点击通知时触发：取出 userInfo 里的 url 并打开浏览器。
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
    didReceiveNotificationResponse:(UNNotificationResponse *)response
             withCompletionHandler:(void (^)(void))completionHandler {
    NSString *url = response.notification.request.content.userInfo[@"url"];
    if (url != nil && [url length] > 0) {
        NSLog(@"aellus: notification clicked -> open %@", url);
        aellusOpenBrowser([url UTF8String]);
    }
    completionHandler();
}

// App 在前台时也允许横幅显示（否则默认不弹）。
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
       willPresentNotification:(UNNotification *)notification
         withCompletionHandler:(void (^)(UNNotificationPresentationOptions))completionHandler {
    completionHandler(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionSound);
}

@end

static AellusNotifyDelegate* gNotifyDelegate = nil;

// 不在主线程就切回主线程执行 block（避免 NSAlert/UN 回调在后台线程碰 UI 而闪退）。
static void onMain(dispatch_block_t block) {
    if ([NSThread isMainThread]) { block(); }
    else { dispatch_async(dispatch_get_main_queue(), block); }
}

// 真正发出一条系统通知（前提是已获得授权）。title/body/url 均为已拷贝的 NSString。
static void reallyPost(NSString* title, NSString* body, NSString* url) {
    UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
    if (center == nil) { NSLog(@"aellus: center nil"); return; }
    UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
    content.title = title;
    content.body = body;
    content.sound = [UNNotificationSound defaultSound];
    content.userInfo = @{@"url": url};
    // 1 秒后展示（triggerWithTimeInterval 要求 > 0）
    UNTimeIntervalNotificationTrigger *trigger =
        [UNTimeIntervalNotificationTrigger triggerWithTimeInterval:1 repeats:NO];
    // 必须用【唯一】identifier：固定 identifier 会让系统去重，第二次启动（旧的未交互通知还在）
    // 时新请求被当作“更新已有通知”而不再弹横幅。每次用 NSUUID 保证新建一条。
    NSString *reqID = [NSUUID UUID].UUIDString;
    UNNotificationRequest *request =
        [UNNotificationRequest requestWithIdentifier:reqID content:content trigger:trigger];
    [center addNotificationRequest:request withCompletionHandler:^(NSError *error) {
        if (error != nil) {
            NSLog(@"aellus: addNotificationRequest failed: %@", error);
        } else {
            NSLog(@"aellus: notification scheduled");
        }
    }];
}

// hasAppBundle 判断当前进程是否运行在 .app bundle 内。
// UNUserNotificationCenter 要求进程有 bundle 身份，裸二进制调用会抛
// NSInternalInconsistencyException (bundleProxyForCurrentProcess is nil) 直接崩溃。
static BOOL hasAppBundle(void) {
    NSString *bundlePath = [[NSBundle mainBundle] bundlePath];
    return bundlePath != nil && [bundlePath hasSuffix:@".app"];
}

void postNotify(const char* title, const char* body, const char* url) {
    // 立刻把 C 字符串拷贝成 NSString（自带内存），避免 Go 侧 free 后 use-after-free。
    NSString *nsTitle = [NSString stringWithUTF8String:(title ? title : "")];
    NSString *nsBody  = [NSString stringWithUTF8String:(body ? body : "")];
    NSString *nsUrl   = [NSString stringWithUTF8String:(url ? url : "")];
    // 裸二进制（非 .app bundle）无法使用 UNUserNotificationCenter，直接跳过通知，
    // 不崩溃。用户通过终端输出的访问地址自行打开浏览器。
    if (!hasAppBundle()) {
        NSLog(@"aellus: not in app bundle, skip system notification");
        return;
    }
    onMain(^{
        UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
        if (center == nil) { NSLog(@"aellus: center nil, skip notification"); return; }
        if (gNotifyDelegate == nil) {
            gNotifyDelegate = [[AellusNotifyDelegate alloc] init];
            center.delegate = gNotifyDelegate;
        }
        [center getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *settings) {
            UNAuthorizationStatus st = settings.authorizationStatus;
            NSLog(@"aellus: auth status=%ld", (long)st);
            onMain(^{
                if (st == UNAuthorizationStatusAuthorized) {
                    reallyPost(nsTitle, nsBody, nsUrl);
                } else if (st == UNAuthorizationStatusNotDetermined) {
                    // 代理类 app 需要先把自身带到前台，授权弹窗才会出现。
                    if (NSApp != nil) { [NSApp activateIgnoringOtherApps:YES]; }
                    [center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
                                          completionHandler:^(BOOL granted, NSError *error) {
                        NSLog(@"aellus: requestAuthorization granted=%d error=%@", granted, error);
                        onMain(^{
                            if (granted) {
                                reallyPost(nsTitle, nsBody, nsUrl);
                            } else {
                                // 用户未允许（或代理类 app 弹不出授权弹窗，系统直接回调
                                // granted=NO）→ 静默跳过。菜单栏图标已是主要交互入口，
                                // 不再用模态 NSAlert 打断用户。
                                NSLog(@"aellus: notification not granted, skip");
                            }
                        });
                    }];
                } else {
                    // denied / provisional / ephemeral → 系统通知不可用，静默跳过。
                    NSLog(@"aellus: auth status=%ld, skip notification", (long)st);
                }
            });
        }];
    });
}
