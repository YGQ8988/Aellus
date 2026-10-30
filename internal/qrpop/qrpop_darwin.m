#import <Cocoa/Cocoa.h>
#import <dispatch/dispatch.h>

// 单个可复用的浮窗：hover 会频繁触发显示/隐藏，没必要反复创建窗口。
static NSWindow *gWindow = nil;
static NSImageView *gView = nil;
static NSData *gLastData = nil;

static void ensureWindow(void) {
  if (gWindow != nil) return;
  NSRect frame = NSMakeRect(0, 0, 276, 276);
  gWindow = [[NSWindow alloc] initWithContentRect:frame
                                       styleMask:NSWindowStyleMaskBorderless
                                         backing:NSBackingStoreBuffered
                                           defer:NO];
  // 高亮中的系统菜单用的是 NSPopUpMenuWindowLevel，浮窗必须压在它上面，
  // 否则会被菜单整块盖住（浮窗紧贴光标，几乎必然与菜单重叠）。
  [gWindow setLevel:NSPopUpMenuWindowLevel + 1];
  [gWindow setOpaque:YES];
  [gWindow setBackgroundColor:[NSColor whiteColor]];
  [gWindow setHasShadow:YES];
  // 绝不能吃掉鼠标事件：浮窗就贴在菜单旁，抢事件会让菜单自身的 hover 失灵。
  [gWindow setIgnoresMouseEvents:YES];
  gView = [[NSImageView alloc] initWithFrame:NSMakeRect(10, 10, 256, 256)];
  [gView setImageScaling:NSImageScaleProportionallyUpOrDown];
  [[gWindow contentView] addSubview:gView];
}

void qrpopShow(const char *bytes, int length) {
  NSData *data = [NSData dataWithBytes:bytes length:length];
  if (data == nil) return;
  dispatch_async(dispatch_get_main_queue(), ^{
    NSImage *image = [[NSImage alloc] initWithData:data];
    if (image == nil) return;
    ensureWindow();
    if (gLastData == nil || ![gLastData isEqualToData:data]) {
      [gView setImage:image];
      gLastData = data;
    }
    // 摆在光标右侧；右边放不下就挪到左侧，并始终夹在可视区域内。
    NSPoint mouse = [NSEvent mouseLocation];
    NSRect visible = [[NSScreen mainScreen] visibleFrame];
    NSSize size = [gWindow frame].size;
    CGFloat x = mouse.x + 18;
    if (x + size.width > NSMaxX(visible)) {
      x = mouse.x - size.width - 18;
    }
    if (x < NSMinX(visible)) x = NSMinX(visible);
    CGFloat y = mouse.y - size.height / 2;
    if (y < NSMinY(visible)) y = NSMinY(visible);
    if (y + size.height > NSMaxY(visible)) y = NSMaxY(visible) - size.height;
    [gWindow setFrameOrigin:NSMakePoint(x, y)];
    // orderFrontRegardless：显示但不激活——本程序是菜单栏 agent，抢焦点会
    // 让前台应用的输入焦点被莫名其妙地夺走。
    [gWindow orderFrontRegardless];
  });
}

void qrpopHide(void) {
  dispatch_async(dispatch_get_main_queue(), ^{
    [gWindow orderOut:nil];
  });
}
