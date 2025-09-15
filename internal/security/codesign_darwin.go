//go:build darwin
// +build darwin

package security

/*
#cgo CFLAGS: -Werror
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>

// Small helpers to safely pull strongly-typed values out of CFDictionary.

static int cfbool_true(CFBooleanRef b) {
	if (b == kCFBooleanTrue) return 1;
	return 0;
}

static char* cfstring_copy_utf8(CFStringRef s) {
	if (!s) return NULL;
	CFIndex length = CFStringGetLength(s);
	CFIndex maxSize = CFStringGetMaximumSizeForEncoding(length, kCFStringEncodingUTF8) + 1;
	char *buffer = (char*)malloc((size_t)maxSize);
	if (!buffer) return NULL;
	if (CFStringGetCString(s, buffer, maxSize, kCFStringEncodingUTF8)) {
		return buffer;
	}
	free(buffer);
	return NULL;
}

static char* cfurl_path_copy_utf8(CFURLRef url) {
	if (!url) return NULL;
	CFStringRef path = CFURLCopyFileSystemPath(url, kCFURLPOSIXPathStyle);
	if (!path) return NULL;
	char* s = cfstring_copy_utf8(path);
	CFRelease(path);
	return s;
}

static char* cfdata_hex(CFDataRef d) {
	if (!d) return NULL;
	CFIndex n = CFDataGetLength(d);
	const UInt8* p = CFDataGetBytePtr(d);
	char* out = (char*)malloc((size_t)(n*2 + 1));
	if (!out) return NULL;
	static const char* hex = "0123456789abcdef";
	for (CFIndex i = 0; i < n; i++) {
		out[2*i  ] = hex[(p[i] >> 4) & 0xF];
		out[2*i+1] = hex[(p[i]     ) & 0xF];
	}
	out[n*2] = '\0';
	return out;
}

static int cfnumber_u32(CFNumberRef num, uint32_t* out) {
	if (!num || !out) return 0;
	return CFNumberGetValue(num, kCFNumberSInt32Type, out);
}

// Queries Security.framework for code signing info of a PID.
// Returns 0 on success, non-zero on failure.
// On success, allocates C strings that the caller (Go) must free with C.free.
static int query_codesign_for_pid(pid_t pid,
	char** outExecPath,
	char** outSigningID,
	char** outTeamID,
	char** outCDHashHex,
	uint32_t* outFlags,
	int* outSigned,
	int* outValid,
	int* outHasEntitlements)
{
	*outExecPath = NULL;
	*outSigningID = NULL;
	*outTeamID = NULL;
	*outCDHashHex = NULL;
	*outFlags = 0;
	*outSigned = 0;
	*outValid = 0;
	*outHasEntitlements = 0;

	CFNumberRef pidNum = CFNumberCreate(NULL, kCFNumberSInt32Type, &pid);
	if (!pidNum) return -1;

	const void* keys[]   = { kSecGuestAttributePid };
	const void* values[] = { pidNum };
	CFDictionaryRef attrs = CFDictionaryCreate(NULL, keys, values, 1,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFRelease(pidNum);

	if (!attrs) return -2;

	SecCodeRef code = NULL;
	OSStatus st = SecCodeCopyGuestWithAttributes(NULL, attrs, kSecCSDefaultFlags, &code);
	CFRelease(attrs);
	if (st != errSecSuccess || !code) {
		return -3;
	}

	// Check validity (cryptographic + trust policy per default flags)
	st = SecCodeCheckValidityWithErrors(code, kSecCSDefaultFlags, NULL, NULL);
	if (st == errSecSuccess) {
		*outValid = 1;
	}

	// Get signing information dictionary
	CFDictionaryRef info = NULL;
	st = SecCodeCopySigningInformation(code, kSecCSSigningInformation, &info);
	if (st != errSecSuccess || !info) {
		CFRelease(code);
		return -4;
	}

	// If we reached here, it is signed in some manner.
	*outSigned = 1;

	// Extract fields
	CFStringRef ident = (CFStringRef)CFDictionaryGetValue(info, kSecCodeInfoIdentifier);
	if (ident) *outSigningID = cfstring_copy_utf8(ident);

	CFStringRef team = (CFStringRef)CFDictionaryGetValue(info, kSecCodeInfoTeamIdentifier);
	if (team) *outTeamID = cfstring_copy_utf8(team);

	CFDataRef cdhash = (CFDataRef)CFDictionaryGetValue(info, kSecCodeInfoCdHashes);
	if (cdhash) *outCDHashHex = cfdata_hex(cdhash);

	CFNumberRef flags = (CFNumberRef)CFDictionaryGetValue(info, kSecCodeInfoFlags);
	if (flags) cfnumber_u32(flags, outFlags);

	CFURLRef mainExe = (CFURLRef)CFDictionaryGetValue(info, kSecCodeInfoMainExecutable);
	if (mainExe) *outExecPath = cfurl_path_copy_utf8(mainExe);

	// Entitlements present?
	CFDictionaryRef ents = (CFDictionaryRef)CFDictionaryGetValue(info, kSecCodeInfoEntitlementsDict);
	if (ents) *outHasEntitlements = 1;

	CFRelease(info);
	CFRelease(code);
	return 0;
}
*/
import "C"
import (
	"errors"
	"unsafe"
)

type csInfo struct {
	ExecutablePath  string
	SigningID       string
	TeamID          string
	CDHashHex       string
	Flags           uint32
	Signed          bool
	ValidSignature  bool
	HasEntitlements bool
}

func getCodeSignatureInfoForPID(pid int) (*csInfo, error) {
	var cExec, cID, cTeam, cHash *C.char
	var flags C.uint32_t
	var signed, valid, hasEnts C.int

	rc := C.query_codesign_for_pid(C.int(pid), &cExec, &cID, &cTeam, &cHash, &flags, &signed, &valid, &hasEnts)
	defer func() {
		if cExec != nil {
			C.free(unsafe.Pointer(cExec))
		}
		if cID != nil {
			C.free(unsafe.Pointer(cID))
		}
		if cTeam != nil {
			C.free(unsafe.Pointer(cTeam))
		}
		if cHash != nil {
			C.free(unsafe.Pointer(cHash))
		}
	}()

	if rc != 0 {
		return nil, errors.New("Security.framework query failed")
	}

	out := &csInfo{
		ExecutablePath:  cstr(cExec),
		SigningID:       cstr(cID),
		TeamID:          cstr(cTeam),
		CDHashHex:       cstr(cHash),
		Flags:           uint32(flags),
		Signed:          signed == 1,
		ValidSignature:  valid == 1,
		HasEntitlements: hasEnts == 1,
	}
	return out, nil
}

func cstr(p *C.char) string {
	if p == nil {
		return ""
	}
	return C.GoString(p)
}
