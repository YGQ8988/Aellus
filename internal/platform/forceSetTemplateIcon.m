// forceSetTemplateIcon.m — compiled by cgo as Objective-C via #cgo directive
// Bypasses systray's internal setIcon path to reliably render menu-bar icon on macOS 11-26.
#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

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
