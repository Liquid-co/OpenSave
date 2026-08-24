//go:build windows

package winreg

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func available() bool { return true }

var hiveKeys = map[string]registry.Key{
	"HKEY_CURRENT_USER":   registry.CURRENT_USER,
	"HKEY_LOCAL_MACHINE":  registry.LOCAL_MACHINE,
	"HKEY_CLASSES_ROOT":   registry.CLASSES_ROOT,
	"HKEY_USERS":          registry.USERS,
	"HKEY_CURRENT_CONFIG": registry.CURRENT_CONFIG,
}

// Type names are written out rather than stored as the numeric constant, so a
// capture stays readable and does not depend on Windows header values staying
// put.
var typeNames = map[uint32]string{
	registry.SZ:        "SZ",
	registry.EXPAND_SZ: "EXPAND_SZ",
	registry.BINARY:    "BINARY",
	registry.DWORD:     "DWORD",
	registry.QWORD:     "QWORD",
	registry.MULTI_SZ:  "MULTI_SZ",
}

var typeValues = func() map[string]uint32 {
	out := make(map[string]uint32, len(typeNames))
	for v, n := range typeNames {
		out[n] = v
	}
	return out
}()

func capture(path string) (Key, bool, error) {
	hive, sub, err := splitHive(path)
	if err != nil {
		return Key{}, false, err
	}
	root, ok := hiveKeys[hive]
	if !ok {
		return Key{}, false, fmt.Errorf("unsupported hive %q", hive)
	}
	k, found, err := captureUnder(root, sub, NormalizePath(path))
	return k, found, err
}

func captureUnder(root registry.Key, sub, fullPath string) (Key, bool, error) {
	h, err := registry.OpenKey(root, sub, registry.READ)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			// A game that has never run has never written its key. That is a
			// correct capture of "nothing here", not a failure to read.
			return Key{}, false, nil
		}
		return Key{}, false, err
	}
	defer h.Close()

	out := Key{Path: fullPath}

	names, err := h.ReadValueNames(-1)
	if err != nil {
		return Key{}, false, err
	}
	for _, name := range names {
		v, err := readValue(h, name)
		if err != nil {
			// One unreadable value does not sink the key: a partial capture
			// that names what it holds beats no capture of the save at all.
			continue
		}
		out.Values = append(out.Values, v)
	}

	subNames, err := h.ReadSubKeyNames(-1)
	if err != nil {
		return out, true, nil // values captured; subkeys unreadable
	}
	for _, s := range subNames {
		child, found, err := captureUnder(root, sub+`\`+s, fullPath+`\`+s)
		if err != nil || !found {
			continue
		}
		out.Subkeys = append(out.Subkeys, child)
	}
	return out, true, nil
}

func readValue(h registry.Key, name string) (Value, error) {
	_, valType, err := h.GetValue(name, nil)
	if err != nil {
		return Value{}, err
	}
	typeName, known := typeNames[valType]
	if !known {
		typeName = "BINARY" // unknown types round-trip as their raw bytes
	}
	v := Value{Name: name, Type: typeName}

	switch valType {
	case registry.SZ, registry.EXPAND_SZ:
		s, _, err := h.GetStringValue(name)
		if err != nil {
			return Value{}, err
		}
		v.Data = s
	case registry.MULTI_SZ:
		ss, _, err := h.GetStringsValue(name)
		if err != nil {
			return Value{}, err
		}
		v.Multi = ss
	case registry.DWORD, registry.QWORD:
		n, _, err := h.GetIntegerValue(name)
		if err != nil {
			return Value{}, err
		}
		v.Data = strconv.FormatUint(n, 10)
	default:
		b, _, err := h.GetBinaryValue(name)
		if err != nil {
			return Value{}, err
		}
		v.Data = base64.StdEncoding.EncodeToString(b)
	}
	return v, nil
}

func restore(k Key) error {
	if strings.TrimSpace(k.Path) == "" {
		return errors.New("a captured key with no path cannot be restored")
	}
	hive, sub, err := splitHive(k.Path)
	if err != nil {
		return err
	}
	root, ok := hiveKeys[hive]
	if !ok {
		return fmt.Errorf("unsupported hive %q", hive)
	}

	h, _, err := registry.CreateKey(root, sub, registry.WRITE)
	if err != nil {
		return fmt.Errorf("creating %s: %w", k.Path, err)
	}
	defer h.Close()

	for _, v := range k.Values {
		if err := writeValue(h, v); err != nil {
			return fmt.Errorf("writing %s\\%s: %w", k.Path, v.Name, err)
		}
	}
	for _, child := range k.Subkeys {
		if err := restore(child); err != nil {
			return err
		}
	}
	return nil
}

func writeValue(h registry.Key, v Value) error {
	switch typeValues[v.Type] {
	case registry.SZ:
		return h.SetStringValue(v.Name, v.Data)
	case registry.EXPAND_SZ:
		return h.SetExpandStringValue(v.Name, v.Data)
	case registry.MULTI_SZ:
		return h.SetStringsValue(v.Name, v.Multi)
	case registry.DWORD:
		n, err := strconv.ParseUint(v.Data, 10, 32)
		if err != nil {
			return err
		}
		return h.SetDWordValue(v.Name, uint32(n))
	case registry.QWORD:
		n, err := strconv.ParseUint(v.Data, 10, 64)
		if err != nil {
			return err
		}
		return h.SetQWordValue(v.Name, n)
	default:
		b, err := base64.StdEncoding.DecodeString(v.Data)
		if err != nil {
			return err
		}
		return h.SetBinaryValue(v.Name, b)
	}
}
