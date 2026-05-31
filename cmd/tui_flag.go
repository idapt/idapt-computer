package cmd

import (
	"os"
	"reflect"
	"strings"
	"unsafe"

	"github.com/idapt/idapt-cli/internal/credential"
	"github.com/idapt/idapt-cli/internal/features"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func readTUICachedAPIKey() string {
	if k := os.Getenv("IDAPT_API_KEY"); k != "" && !strings.HasPrefix(k, "mk_") {
		return k
	}
	path, err := credential.DefaultPath()
	if err != nil {
		return ""
	}
	creds, err := credential.Load(path)
	if err != nil {
		return ""
	}
	return creds.APIKey
}

func isTUIEnabledFromCache() bool {
	cachePath, err := features.DefaultCachePath()
	if err != nil || cachePath == "" {
		return false
	}
	cached := features.LoadFromCache(cachePath, readTUICachedAPIKey())
	if cached == nil {
		return false
	}
	return cached.IsEnabled(features.FlagTUI)
}

func registerTUISurface(root, chat, chatSend *cobra.Command, enabled bool) {
	if !enabled {
		return
	}

	if root != nil && !containsCommand(root, tuiCmd) {
		root.AddCommand(tuiCmd)
	}
	if chat != nil && !containsCommand(chat, chatAskCmd) {
		chat.AddCommand(chatAskCmd)
	}

	if root != nil && root.PersistentFlags().Lookup("prompt") == nil {
		root.PersistentFlags().StringP(
			"prompt", "p", "",
			"one-shot prompt; bypasses the TUI and streams the response to stdout",
		)
	}

	if chatSend != nil && chatSend.Flags().Lookup("stream") == nil {
		chatSend.Flags().Bool(
			"stream", false,
			"Stream the response via SSE (mutually exclusive with --no-wait)",
		)
	}
}

func containsCommand(parent, child *cobra.Command) bool {
	if parent == nil || child == nil {
		return false
	}
	for _, c := range parent.Commands() {
		if c == child {
			return true
		}
	}
	return false
}

func resetTUISurface(root, chat, chatSend *cobra.Command) {
	if root != nil && containsCommand(root, tuiCmd) {
		root.RemoveCommand(tuiCmd)
	}
	if chat != nil && containsCommand(chat, chatAskCmd) {
		chat.RemoveCommand(chatAskCmd)
	}
	if root != nil {
		removeFlag(root.PersistentFlags(), "prompt", "p")
	}
	if chatSend != nil {
		removeFlag(chatSend.Flags(), "stream", "")
	}
}

func removeFlag(fs *pflag.FlagSet, name, shorthand string) {
	if fs == nil {
		return
	}
	if fs.Lookup(name) == nil && (shorthand == "" || fs.ShorthandLookup(shorthand) == nil) {
		return
	}

	v := reflect.ValueOf(fs).Elem()

	formal := exposeField(v.FieldByName("formal"))
	if formal.IsValid() && formal.Kind() == reflect.Map {
		for _, key := range formal.MapKeys() {
			if key.String() == name {
				formal.SetMapIndex(key, reflect.Value{})
			}
		}
	}
	if shorthand != "" {
		shorthands := exposeField(v.FieldByName("shorthands"))
		if shorthands.IsValid() && shorthands.Kind() == reflect.Map {
			for _, key := range shorthands.MapKeys() {
				if key.Uint() == uint64(shorthand[0]) {
					shorthands.SetMapIndex(key, reflect.Value{})
				}
			}
		}
	}
	ordered := exposeField(v.FieldByName("orderedFormal"))
	if ordered.IsValid() && ordered.Kind() == reflect.Slice {
		kept := reflect.MakeSlice(ordered.Type(), 0, ordered.Len())
		for i := 0; i < ordered.Len(); i++ {
			f := ordered.Index(i)
			if f.Kind() == reflect.Ptr && !f.IsNil() {
				nameField := f.Elem().FieldByName("Name")
				if nameField.IsValid() && nameField.String() == name {
					continue
				}
			}
			kept = reflect.Append(kept, f)
		}
		ordered.Set(kept)
	}
}

func exposeField(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v
	}
	return reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem()
}
