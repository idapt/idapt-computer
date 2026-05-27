package commands

type VerbSpec struct {
	Name     string // canonical verb name (without leading /)
	ArgsHint string // e.g. "<path>" or "[id]"
	Short    string // one-line description
	Aliases  []string
}

var Registry = func() map[string]VerbSpec {
	verbs := []VerbSpec{
		{Name: "help", Short: "Show keybindings + slash commands"},
		{Name: "new", Aliases: []string{"clear"}, Short: "End current chat, open a fresh one"},
		{Name: "quit", Aliases: []string{"exit"}, Short: "Quit and save state"},
		{Name: "model", ArgsHint: "[id]", Short: "Switch model; no arg opens picker"},
		{Name: "agent", ArgsHint: "[name|id]", Short: "Switch agent; no arg opens picker"},
		{Name: "workspace", ArgsHint: "[slug|id]", Short: "Switch workspace; no arg opens picker"},
		{Name: "file", ArgsHint: "<path>", Short: "Attach file to next message"},
		{Name: "files", Short: "List attached files"},
		{Name: "unfile", ArgsHint: "<path>", Short: "Remove an attached file"},
		{Name: "regen", Short: "Regenerate last assistant response"},
		{Name: "edit", Short: "Edit last user message in composer"},
		{Name: "copy", Short: "Copy last assistant message (OSC52)"},
		{Name: "theme", ArgsHint: "[auto|light|dark]", Short: "Cycle / set the color theme"},
		{Name: "menu", Short: "Open the command palette (clickable + arrow-nav)"},
	}
	m := make(map[string]VerbSpec, len(verbs)*2)
	for _, v := range verbs {
		m[v.Name] = v
		for _, alias := range v.Aliases {
			m[alias] = v
		}
	}
	return m
}()

func CanonicalVerbs() []VerbSpec {
	order := []string{
		"help", "menu", "new", "quit",
		"model", "agent", "workspace",
		"file", "files", "unfile",
		"regen", "edit", "copy",
		"theme",
	}
	out := make([]VerbSpec, 0, len(order))
	for _, name := range order {
		if v, ok := Registry[name]; ok && v.Name == name {
			out = append(out, v)
		}
	}
	return out
}

func VisibleVerbs() []VerbSpec {
	out := CanonicalVerbs()
	canonical := map[string]VerbSpec{}
	for _, v := range out {
		canonical[v.Name] = v
	}
	for i, v := range out {
		_ = i
		_ = v
	}
	aliases := []string{"clear", "exit"}
	for _, alias := range aliases {
		if v, ok := Registry[alias]; ok && v.Name != alias {
			out = append(out, VerbSpec{
				Name:     alias,
				ArgsHint: v.ArgsHint,
				Short:    "alias for /" + v.Name,
			})
		}
	}
	return out
}
