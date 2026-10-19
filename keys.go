package footswitch

import "fmt"

// keyDef ties a Linux evdev key name and code to the USB HID keyboard usage
// (page 0x07) the pedal stores in its flash.
type keyDef struct {
	name string
	code uint16
	hid  byte
}

// keyTable lists every key a pedal slot can store as a plain key. Modifier
// keys are left out: the pedal stores them in a separate modifier byte.
var keyTable = []keyDef{
	{"KEY_A", 30, 0x04}, {"KEY_B", 48, 0x05}, {"KEY_C", 46, 0x06}, {"KEY_D", 32, 0x07},
	{"KEY_E", 18, 0x08}, {"KEY_F", 33, 0x09}, {"KEY_G", 34, 0x0a}, {"KEY_H", 35, 0x0b},
	{"KEY_I", 23, 0x0c}, {"KEY_J", 36, 0x0d}, {"KEY_K", 37, 0x0e}, {"KEY_L", 38, 0x0f},
	{"KEY_M", 50, 0x10}, {"KEY_N", 49, 0x11}, {"KEY_O", 24, 0x12}, {"KEY_P", 25, 0x13},
	{"KEY_Q", 16, 0x14}, {"KEY_R", 19, 0x15}, {"KEY_S", 31, 0x16}, {"KEY_T", 20, 0x17},
	{"KEY_U", 22, 0x18}, {"KEY_V", 47, 0x19}, {"KEY_W", 17, 0x1a}, {"KEY_X", 45, 0x1b},
	{"KEY_Y", 21, 0x1c}, {"KEY_Z", 44, 0x1d},
	{"KEY_1", 2, 0x1e}, {"KEY_2", 3, 0x1f}, {"KEY_3", 4, 0x20}, {"KEY_4", 5, 0x21},
	{"KEY_5", 6, 0x22}, {"KEY_6", 7, 0x23}, {"KEY_7", 8, 0x24}, {"KEY_8", 9, 0x25},
	{"KEY_9", 10, 0x26}, {"KEY_0", 11, 0x27},
	{"KEY_ENTER", 28, 0x28}, {"KEY_ESC", 1, 0x29}, {"KEY_BACKSPACE", 14, 0x2a},
	{"KEY_TAB", 15, 0x2b}, {"KEY_SPACE", 57, 0x2c}, {"KEY_MINUS", 12, 0x2d},
	{"KEY_EQUAL", 13, 0x2e}, {"KEY_LEFTBRACE", 26, 0x2f}, {"KEY_RIGHTBRACE", 27, 0x30},
	{"KEY_BACKSLASH", 43, 0x31}, {"KEY_SEMICOLON", 39, 0x33}, {"KEY_APOSTROPHE", 40, 0x34},
	{"KEY_GRAVE", 41, 0x35}, {"KEY_COMMA", 51, 0x36}, {"KEY_DOT", 52, 0x37},
	{"KEY_SLASH", 53, 0x38}, {"KEY_CAPSLOCK", 58, 0x39},
	{"KEY_F1", 59, 0x3a}, {"KEY_F2", 60, 0x3b}, {"KEY_F3", 61, 0x3c}, {"KEY_F4", 62, 0x3d},
	{"KEY_F5", 63, 0x3e}, {"KEY_F6", 64, 0x3f}, {"KEY_F7", 65, 0x40}, {"KEY_F8", 66, 0x41},
	{"KEY_F9", 67, 0x42}, {"KEY_F10", 68, 0x43}, {"KEY_F11", 87, 0x44}, {"KEY_F12", 88, 0x45},
	{"KEY_SYSRQ", 99, 0x46}, {"KEY_SCROLLLOCK", 70, 0x47}, {"KEY_PAUSE", 119, 0x48},
	{"KEY_INSERT", 110, 0x49}, {"KEY_HOME", 102, 0x4a}, {"KEY_PAGEUP", 104, 0x4b},
	{"KEY_DELETE", 111, 0x4c}, {"KEY_END", 107, 0x4d}, {"KEY_PAGEDOWN", 109, 0x4e},
	{"KEY_RIGHT", 106, 0x4f}, {"KEY_LEFT", 105, 0x50}, {"KEY_DOWN", 108, 0x51},
	{"KEY_UP", 103, 0x52}, {"KEY_NUMLOCK", 69, 0x53},
	{"KEY_KPSLASH", 98, 0x54}, {"KEY_KPASTERISK", 55, 0x55}, {"KEY_KPMINUS", 74, 0x56},
	{"KEY_KPPLUS", 78, 0x57}, {"KEY_KPENTER", 96, 0x58}, {"KEY_KP1", 79, 0x59},
	{"KEY_KP2", 80, 0x5a}, {"KEY_KP3", 81, 0x5b}, {"KEY_KP4", 75, 0x5c}, {"KEY_KP5", 76, 0x5d},
	{"KEY_KP6", 77, 0x5e}, {"KEY_KP7", 71, 0x5f}, {"KEY_KP8", 72, 0x60}, {"KEY_KP9", 73, 0x61},
	{"KEY_KP0", 82, 0x62}, {"KEY_KPDOT", 83, 0x63}, {"KEY_102ND", 86, 0x64},
	{"KEY_COMPOSE", 127, 0x65}, {"KEY_POWER", 116, 0x66}, {"KEY_KPEQUAL", 117, 0x67},
	{"KEY_F13", 183, 0x68}, {"KEY_F14", 184, 0x69}, {"KEY_F15", 185, 0x6a}, {"KEY_F16", 186, 0x6b},
	{"KEY_F17", 187, 0x6c}, {"KEY_F18", 188, 0x6d}, {"KEY_F19", 189, 0x6e}, {"KEY_F20", 190, 0x6f},
	{"KEY_F21", 191, 0x70}, {"KEY_F22", 192, 0x71}, {"KEY_F23", 193, 0x72}, {"KEY_F24", 194, 0x73},
	{"KEY_OPEN", 134, 0x74}, {"KEY_HELP", 138, 0x75}, {"KEY_PROPS", 130, 0x76},
	{"KEY_FRONT", 132, 0x77}, {"KEY_STOP", 128, 0x78}, {"KEY_AGAIN", 129, 0x79},
	{"KEY_UNDO", 131, 0x7a}, {"KEY_CUT", 137, 0x7b}, {"KEY_COPY", 133, 0x7c},
	{"KEY_PASTE", 135, 0x7d}, {"KEY_FIND", 136, 0x7e}, {"KEY_MUTE", 113, 0x7f},
	{"KEY_VOLUMEUP", 115, 0x80}, {"KEY_VOLUMEDOWN", 114, 0x81},
}

// modifierNames are evdev keys that exist but can't be stored as a pedal's key.
var modifierNames = map[string]uint16{
	"KEY_LEFTCTRL": 29, "KEY_LEFTSHIFT": 42, "KEY_LEFTALT": 56, "KEY_LEFTMETA": 125,
	"KEY_RIGHTCTRL": 97, "KEY_RIGHTSHIFT": 54, "KEY_RIGHTALT": 100, "KEY_RIGHTMETA": 126,
}

var (
	keysByName = indexKeys(func(k keyDef) string { return k.name })
	keysByCode = indexKeys(func(k keyDef) uint16 { return k.code })
	keysByHID  = indexKeys(func(k keyDef) byte { return k.hid })
)

func indexKeys[K comparable](key func(keyDef) K) map[K]keyDef {
	out := make(map[K]keyDef, len(keyTable))
	for _, k := range keyTable {
		out[key(k)] = k
	}
	return out
}

// lookupKeycode resolves a config keycode to a key the pedal can store.
func lookupKeycode(name string) (keyDef, error) {
	if k, ok := keysByName[name]; ok {
		return k, nil
	}
	if _, ok := modifierNames[name]; ok {
		return keyDef{}, fmt.Errorf("%q is a modifier key, which the pedal can't store as its key", name)
	}
	return keyDef{}, fmt.Errorf("%q is not a known evdev key name with a USB HID equivalent (for example KEY_F13)", name)
}

// codeName names an evdev key code for logs.
func codeName(code uint16) string {
	if k, ok := keysByCode[code]; ok {
		return k.name
	}
	for name, c := range modifierNames {
		if c == code {
			return name
		}
	}
	return fmt.Sprintf("key code %d", code)
}
