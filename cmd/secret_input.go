package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)
func stdinIsTTY() bool {
	return isatty.IsTerminal(os.Stdin.Fd())
}

func readSecretStdin(in io.Reader) (string, error) {
	data, err := io.ReadAll(in)
	if err != nil {
		return "", fmt.Errorf("reading secret from stdin: %w", err)
	}
	return strings.TrimRight(string(data), "\r\n"), nil
}

func promptNoEcho(label string, errOut io.Writer) (string, error) {
	fmt.Fprint(errOut, label)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(errOut) // newline the silent input swallowed
	if err != nil {
		return "", fmt.Errorf("reading password: %w", err)
	}
	return string(b), nil
}

func promptLine(in io.Reader, errOut io.Writer, label string) (string, error) {
	fmt.Fprint(errOut, label)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func warnInsecureFlag(errOut io.Writer, flag, alternative string) {
	fmt.Fprintf(errOut,
		"WARNING: passing %s on the command line is insecure — it can be recovered from your shell history and the process list. %s.\n",
		flag, alternative)
}

func resolvePairToken(cmd *cobra.Command, tokenFlag string, tokenStdin bool) (string, error) {
	if tokenStdin {
		if tokenFlag != "" {
			return "", errors.New("--token and --token-stdin are mutually exclusive")
		}
		token, err := readSecretStdin(cmd.InOrStdin())
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(token) == "" {
			return "", errors.New("--token-stdin was set but no registration token was provided on stdin")
		}
		return token, nil
	}
	if tokenFlag != "" {
		warnInsecureFlag(cmd.ErrOrStderr(), "--token",
			"Pipe it via --token-stdin or set the IDAPT_TOKEN env var instead")
		return tokenFlag, nil
	}
	return strings.TrimSpace(os.Getenv("IDAPT_TOKEN")), nil
}
