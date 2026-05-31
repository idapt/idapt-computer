package cmd
import (
	"github.com/idapt/idapt-cli/internal/api"
	"github.com/idapt/idapt-cli/internal/cmdutil"
	"github.com/idapt/idapt-cli/internal/output"
	"github.com/spf13/cobra"
)

var computerUseCmd = &cobra.Command{
	Use:     "computer-use",
	Aliases: []string{"cu"},
	Short:   "Control a computer's desktop (screenshot, mouse, keyboard)",
	Long: `Pure image + input GUI control of a cloud computer's desktop — the same
capability as the agent computer-use tool (modeled on Anthropic computer use).

Take a screenshot, read where things are, then click/type/scroll at pixel
coordinates. Screenshots are saved to Drive and the result returns their
resource link + project + path.`,
	Annotations: map[string]string{
		"instructions": `# computer-use — instructions

Drive a graphical desktop the way a person would: look, then act, then look
again. Pure pixels — there is no accessibility tree, so the screenshot is your
only source of truth.

## The loop
1. ` + "`screenshot`" + ` to see the screen (saved to Drive; result has its link + path).
2. Read it, decide the single next action.
3. Perform ONE action (left-click, type, key, scroll, …).
4. ` + "`screenshot`" + ` again to confirm before the next action.

## Coordinates
Pixels in the most recent screenshot, origin top-left. Always screenshot before
clicking — the screen drifts after even one action.

## Prerequisite
The computer needs a desktop session (X server + xdotool + scrot on Linux). If
you get a runtime-unavailable error, install a desktop environment first.

## Keyboard
- ` + "`type`" + ` for literal text; ` + "`key`" + ` for chords (Return, ctrl+s, alt+Tab).

## Anti-patterns
- Don't click without a fresh screenshot.
- Don't batch many actions blind — one action, then re-screenshot.`,
	},
}

var desktopColumns = []output.Column{
	{Header: "ACTION", Field: "action"},
	{Header: "RES LINK", Field: "resLink"},
	{Header: "PATH", Field: "path"},
	{Header: "WIDTH", Field: "width"},
	{Header: "HEIGHT", Field: "height"},
	{Header: "X", Field: "x"},
	{Header: "Y", Field: "y"},
}

func runDesktopVerb(action string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		f := cmdutil.FactoryFromCmd(cmd)
		client, err := f.APIClient()
		if err != nil {
			return err
		}
		id, err := resolveComputer(cmd, f, args[0])
		if err != nil {
			return err
		}

		body := map[string]interface{}{"action": action}
		addChangedInt(cmd, body, "x", "x")
		addChangedInt(cmd, body, "y", "y")
		addChangedInt(cmd, body, "start-x", "startX")
		addChangedInt(cmd, body, "start-y", "startY")
		addChangedInt(cmd, body, "amount", "scrollAmount")
		addChangedString(cmd, body, "text", "text")
		addChangedString(cmd, body, "direction", "scrollDirection")
		addChangedString(cmd, body, "folder", "folderId")
		addChangedFloat(cmd, body, "duration", "duration")

		var resp api.V1ItemResponse
		if err := client.Post(cmd.Context(), "/api/v1/computers/"+id+"/desktop", body, &resp); err != nil {
			return err
		}
		return f.Formatter().WriteItem(resp.Data, desktopColumns)
	}
}

func addChangedInt(cmd *cobra.Command, body map[string]interface{}, flag, field string) {
	if cmd.Flags().Lookup(flag) != nil && cmd.Flags().Changed(flag) {
		v, _ := cmd.Flags().GetInt(flag)
		body[field] = v
	}
}

func addChangedFloat(cmd *cobra.Command, body map[string]interface{}, flag, field string) {
	if cmd.Flags().Lookup(flag) != nil && cmd.Flags().Changed(flag) {
		v, _ := cmd.Flags().GetFloat64(flag)
		body[field] = v
	}
}

func addChangedString(cmd *cobra.Command, body map[string]interface{}, flag, field string) {
	if cmd.Flags().Lookup(flag) != nil && cmd.Flags().Changed(flag) {
		v, _ := cmd.Flags().GetString(flag)
		body[field] = v
	}
}

func newDesktopVerb(action, short string) *cobra.Command {
	return &cobra.Command{
		Use:   action + " <id-or-name>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE:  runDesktopVerb(action),
	}
}

func init() {
	screenshot := newDesktopVerb("screenshot", "Capture the desktop (saved to Drive)")
	screenshot.Flags().String("folder", "", "Drive folder id to save into (default: workspace root)")

	cursorPosition := newDesktopVerb("cursor-position", "Read the current mouse position")

	mouseMove := newDesktopVerb("mouse-move", "Move the mouse to x,y")
	addCoordFlags(mouseMove)

	leftClick := newDesktopVerb("left-click", "Left-click at x,y (optional --text modifier)")
	addClickFlags(leftClick)
	rightClick := newDesktopVerb("right-click", "Right-click at x,y")
	addClickFlags(rightClick)
	middleClick := newDesktopVerb("middle-click", "Middle-click at x,y")
	addClickFlags(middleClick)
	doubleClick := newDesktopVerb("double-click", "Double-click at x,y")
	addClickFlags(doubleClick)
	tripleClick := newDesktopVerb("triple-click", "Triple-click at x,y")
	addClickFlags(tripleClick)

	leftMouseDown := newDesktopVerb("left-mouse-down", "Press the left mouse button")
	addCoordFlags(leftMouseDown)
	leftMouseUp := newDesktopVerb("left-mouse-up", "Release the left mouse button")
	addCoordFlags(leftMouseUp)

	drag := newDesktopVerb("left-click-drag", "Drag from start-x,start-y to x,y")
	drag.Flags().Int("start-x", 0, "Drag start X")
	drag.Flags().Int("start-y", 0, "Drag start Y")
	addCoordFlags(drag)

	scroll := newDesktopVerb("scroll", "Scroll at x,y in a direction")
	addCoordFlags(scroll)
	scroll.Flags().String("direction", "", "up | down | left | right (required)")
	scroll.Flags().Int("amount", 3, "Number of wheel clicks")
	scroll.Flags().String("text", "", "Optional modifier held while scrolling")

	keyCmd := newDesktopVerb("key", "Press a key chord (e.g. Return, ctrl+s)")
	keyCmd.Flags().String("text", "", "Key chord in xdotool syntax (required)")

	typeCmd := newDesktopVerb("type", "Type literal text at the cursor")
	typeCmd.Flags().String("text", "", "Text to type (required)")

	holdKey := newDesktopVerb("hold-key", "Hold a key chord for duration seconds")
	holdKey.Flags().String("text", "", "Key chord to hold (required)")
	holdKey.Flags().Float64("duration", 1, "Seconds to hold")

	waitCmd := newDesktopVerb("wait", "Wait for duration seconds")
	waitCmd.Flags().Float64("duration", 1, "Seconds to wait")

	for _, c := range []*cobra.Command{
		screenshot, cursorPosition, mouseMove,
		leftClick, rightClick, middleClick, doubleClick, tripleClick,
		leftMouseDown, leftMouseUp, drag, scroll,
		keyCmd, typeCmd, holdKey, waitCmd,
	} {
		computerUseCmd.AddCommand(c)
	}

	rootCmd.AddCommand(computerUseCmd)
}

func addCoordFlags(cmd *cobra.Command) {
	cmd.Flags().Int("x", 0, "X coordinate (pixels)")
	cmd.Flags().Int("y", 0, "Y coordinate (pixels)")
}

func addClickFlags(cmd *cobra.Command) {
	addCoordFlags(cmd)
	cmd.Flags().String("text", "", "Optional modifier held during the click (e.g. ctrl)")
}
